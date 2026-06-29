package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// JSON-RPC framing for the MCP stdio transport. We accept and emit one
// JSON object per line. The buffer is sized so a single large
// `tools/call` frame (e.g. an agent passing a long quickstart payload)
// does not trip bufio.Scanner's default 64 KiB line cap.
const (
	mcpScannerInitialBuffer = 64 * 1024
	mcpScannerMaxBuffer     = 4 * 1024 * 1024 // 4 MiB
	mcpProtocolVersion      = "2024-11-05"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      any          `json:"id,omitempty"`
	Result  any          `json:"result,omitempty"`
	Error   *mcpRPCError `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Meta      mcpCallMeta    `json:"_meta"`
}

type mcpCallMeta struct {
	ProgressToken any `json:"progressToken"`
}

func (a *App) buildMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run Agora CLI as a local MCP server",
		Long: `Expose Agora CLI tools to MCP-capable agents.

Use this when an MCP client (Cursor, Claude Code, Windsurf, custom) wants to drive Agora workflows directly. The full Agora command surface is exposed as MCP tools so agents can authenticate, discover, manage projects, scaffold quickstarts, and run readiness checks without shelling out.

Notes for agents:
- Long-running tools (init, quickstart create, project create) emit MCP progress notifications when the tools/call params include _meta.progressToken.
- Shell commands still use NDJSON progress (` + "`agora <command> --json`" + `); MCP uses JSON-RPC notifications so stdio framing remains valid.
- ` + "`agora.auth.login`" + ` is intentionally not exposed because OAuth requires an interactive browser. Run ` + "`agora login`" + ` once on the host before starting the MCP server.
- All tools return JSON-stringified payloads in the standard MCP ` + "`content[0].text`" + ` slot.`,
		Example: example(`
  agora mcp serve
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	var transport string
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve Agora CLI tools over MCP",
		Example: example(`
  agora mcp serve
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if transport != "stdio" {
				return fmt.Errorf("unsupported MCP transport %q; use stdio", transport)
			}
			return a.serveMCP(cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	serve.Flags().StringVar(&transport, "transport", "stdio", "MCP transport: stdio")
	_ = serve.Flags().MarkHidden("transport")
	cmd.AddCommand(serve)
	return cmd
}

func (a *App) serveMCP(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	// Default Scanner has a 64 KiB line cap. Larger payloads (a quickstart
	// create with custom args, a doctor result echoed back to the client)
	// can exceed that, so widen the cap up to 4 MiB.
	scanner.Buffer(make([]byte, mcpScannerInitialBuffer), mcpScannerMaxBuffer)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = encoder.Encode(mcpResponse{JSONRPC: "2.0", Error: &mcpRPCError{Code: -32700, Message: err.Error()}})
			continue
		}
		// JSON-RPC 2.0: a frame without an ID is a notification and
		// MUST NOT receive a response. We respect that for any method,
		// not just notifications/*.
		isNotification := req.ID == nil
		result, err := a.handleMCPRequest(req, encoder)
		if isNotification {
			continue
		}
		resp := mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
		if err != nil {
			resp.Result = nil
			resp.Error = &mcpRPCError{Code: -32000, Message: err.Error()}
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp transport read error: %w", err)
	}
	return nil
}

func (a *App) handleMCPRequest(req mcpRequest, encoder *json.Encoder) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{"name": "agora-cli", "version": version},
		}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools()}, nil
	case "tools/call":
		var call mcpToolCall
		if err := json.Unmarshal(req.Params, &call); err != nil {
			return nil, err
		}
		data, err := a.callMCPTool(call.Name, call.Arguments, makeMCPProgressEmitter(encoder, call.Meta.ProgressToken))
		if err != nil {
			return nil, err
		}
		raw, _ := json.MarshalIndent(data, "", "  ")
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(raw)}}}, nil
	case "ping":
		return map[string]any{}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP method %q", req.Method)
	}
}

// mcpTools enumerates the full tool surface. New tools should be added
// here AND wired in callMCPTool below. Keep names dot-namespaced under
// agora.<group>[.<sub>] to match the CLI command tree.
func mcpTools() []map[string]any {
	return []map[string]any{
		// Discovery and metadata
		mcpTool("agora.version", "Show CLI build information", nil),
		mcpTool("agora.introspect", "Emit machine-readable command metadata for the entire CLI", nil),

		// Authentication
		mcpTool("agora.auth.status", "Inspect local Agora authentication state", nil),
		mcpTool("agora.auth.logout", "Clear the local Agora session", nil),

		// Configuration
		mcpTool("agora.config.path", "Show the path to the persisted CLI config file", nil),
		mcpTool("agora.config.get", "Read persisted CLI defaults", nil),

		// Telemetry
		mcpTool("agora.telemetry.status", "Show telemetry status and DO_NOT_TRACK detection", nil),

		// Upgrade guidance
		mcpTool("agora.upgrade.check", "Resolve the latest release and report what would happen", nil),

		// Projects
		mcpTool("agora.project.list", "List Agora projects", map[string]string{"keyword": "string", "page": "number", "pageSize": "number"}),
		mcpTool("agora.project.show", "Show one Agora project", map[string]string{"project": "string"}),
		mcpTool("agora.project.use", "Select the current Agora project", map[string]string{"project": "string"}),
		mcpTool("agora.project.create", "Create a new Agora project and optionally enable features", map[string]string{
			"name":           "string",
			"template":       "string",
			"features":       "array",
			"rtmDataCenter":  "string",
			"idempotencyKey": "string",
		}),
		mcpTool("agora.project.doctor", "Run project readiness diagnostics", map[string]string{"project": "string", "feature": "string", "deep": "boolean"}),
		mcpTool("agora.project.env", "Render project environment variable values", map[string]string{"project": "string", "withSecrets": "boolean"}),
		mcpTool("agora.project.env_write", "Write project env values to a dotenv file in a workspace", map[string]string{"workspaceDir": "string", "project": "string", "template": "string"}),

		// Project features
		mcpTool("agora.project.feature.list", "List feature status for a project", map[string]string{"project": "string"}),
		mcpTool("agora.project.feature.status", "Show one feature status", map[string]string{"feature": "string", "project": "string"}),
		mcpTool("agora.project.feature.enable", "Enable one feature for a project", map[string]string{"feature": "string", "project": "string"}),

		// Project webhooks
		mcpTool("agora.project.webhook.events", "List available webhook events for a feature", map[string]string{"feature": "string"}),
		mcpTool("agora.project.webhook.list", "List webhook configurations for a project feature", map[string]string{"feature": "string", "project": "string"}),
		mcpTool("agora.project.webhook.show", "Show one webhook configuration", map[string]string{
			"configId":   "integer",
			"feature":    "string",
			"project":    "string",
			"withSecret": "boolean",
		}),
		mcpTool("agora.project.webhook.create", "Create a webhook configuration", map[string]string{
			"feature":        "string",
			"project":        "string",
			"url":            "string",
			"events":         "array",
			"secret":         "string",
			"deliveryRegion": "string",
		}),
		mcpTool("agora.project.webhook.update", "Update a webhook configuration", map[string]string{
			"configId":       "integer",
			"feature":        "string",
			"project":        "string",
			"url":            "string",
			"events":         "array",
			"deliveryRegion": "string",
			"enabled":        "boolean",
		}),
		mcpTool("agora.project.webhook.delete", "Delete a webhook configuration", map[string]string{
			"configId": "integer",
			"feature":  "string",
			"project":  "string",
			"confirm":  "boolean",
		}),

		// Quickstart
		mcpTool("agora.quickstart.list", "List quickstart templates", nil),
		mcpTool("agora.quickstart.create", "Clone a quickstart and optionally bind a project", map[string]string{
			"name":     "string",
			"template": "string",
			"project":  "string",
			"ref":      "string",
			"dir":      "string",
		}),
		mcpTool("agora.quickstart.env_write", "Write env values into a previously-cloned quickstart", map[string]string{"dir": "string", "template": "string", "project": "string"}),

		// Init: the recommended end-to-end flow
		mcpTool("agora.init", "Create or bind a project, clone a quickstart, and write env in one call", map[string]string{
			"name":          "string",
			"template":      "string",
			"project":       "string",
			"newProject":    "boolean",
			"rtmDataCenter": "string",
			"features":      "array",
		}),
	}
}

// mcpTool builds an MCP tool descriptor with a JSON-Schema-ish input
// shape. Properties is a {key: type} map; pass nil for tools that take
// no arguments.
func mcpTool(name, description string, properties map[string]string) map[string]any {
	schemaProps := map[string]any{}
	for key, typ := range properties {
		schemaProps[key] = map[string]any{"type": typ}
	}
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           schemaProps,
			"additionalProperties": false,
		},
	}
}

func makeMCPProgressEmitter(encoder *json.Encoder, token any) progressEmitter {
	if encoder == nil || token == nil {
		return nil
	}
	seq := 0
	return func(stage, message string, fields map[string]any) {
		seq++
		params := map[string]any{
			"progressToken": token,
			"progress":      seq,
			"stage":         stage,
			"message":       message,
			"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		}
		for k, v := range fields {
			if _, reserved := params[k]; reserved {
				continue
			}
			params[k] = v
		}
		_ = encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/progress",
			"params":  params,
		})
	}
}

// callMCPTool dispatches a tool name to the underlying App method. New
// tools must be added here in addition to mcpTools(); the switch
// returns an explicit error for unknown names so agents see a clear
// `unknown MCP tool` response.
//
// Important: every path here that calls into a long-running command
// (init, quickstart create) MUST pass non-os.Stdin / non-os.Stderr
// readers and writers so the MCP transport stream (which IS os.Stdin
// when serving over stdio) is never read as if it were user input. We
// pass bytes.NewReader(nil) and io.Discard explicitly to enforce the
// no-stdin contract at the call site.
func (a *App) callMCPTool(name string, args map[string]any, progress progressEmitter) (any, error) {
	switch name {

	case "agora.version":
		return versionInfo(), nil

	case "agora.introspect":
		return buildIntrospectionData(a.root), nil

	case "agora.auth.status":
		return a.authStatus()

	case "agora.auth.logout":
		return a.logout()

	case "agora.config.path":
		path, err := resolveConfigFilePath(a.env)
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": path}, nil

	case "agora.config.get":
		return a.cfg, nil

	case "agora.telemetry.status":
		path, err := resolveConfigFilePath(a.env)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"action":     "status",
			"configPath": path,
			"doNotTrack": strings.TrimSpace(a.env["DO_NOT_TRACK"]) != "",
			"enabled":    a.cfg.TelemetryEnabled && strings.TrimSpace(a.env["DO_NOT_TRACK"]) == "",
		}, nil

	case "agora.upgrade.check":
		return a.performSelfUpdate(true)

	case "agora.project.list":
		page := intArg(args, "page", 1)
		pageSize := intArg(args, "pageSize", 20)
		res, err := a.listProjects(stringArg(args, "keyword"), page, pageSize)
		if err != nil {
			return nil, err
		}
		return map[string]any{"items": res.Items, "page": res.Page, "pageSize": res.PageSize, "total": res.Total}, nil

	case "agora.project.show":
		return a.projectShow(stringArg(args, "project"))

	case "agora.project.use":
		project := stringArg(args, "project")
		if project == "" {
			return nil, errors.New("project is required")
		}
		return a.projectUse(project)

	case "agora.project.create":
		name := stringArg(args, "name")
		if name == "" {
			return nil, &cliError{Message: "project name is required", Code: "PROJECT_NAME_REQUIRED"}
		}
		rtmDataCenter, err := normalizeRTMDataCenter(stringArg(args, "rtmDataCenter"))
		if err != nil {
			return nil, err
		}
		features := stringSliceArg(args, "features")
		progress.emit("project:create", "Creating Agora project", map[string]any{"projectName": name, "features": projectCreateFeatures(stringArg(args, "template"), features)})
		result, err := a.projectCreate(
			name,
			stringArg(args, "template"),
			features,
			rtmDataCenter,
			stringArg(args, "idempotencyKey"),
		)
		if err == nil {
			progress.emit("project:created", "Agora project ready", map[string]any{"projectId": result["projectId"], "projectName": result["projectName"]})
		}
		return result, err

	case "agora.project.doctor":
		return a.projectDoctor(stringArg(args, "project"), defaultString(stringArg(args, "feature"), "convoai"), boolArg(args, "deep", false)), nil

	case "agora.project.env":
		return a.projectEnvValues(stringArg(args, "project"), boolArg(args, "withSecrets", false))

	case "agora.project.env_write":
		return a.quickstartEnvWrite(defaultString(stringArg(args, "workspaceDir"), "."), stringArg(args, "template"), stringArg(args, "project"))

	case "agora.project.feature.list":
		target, err := a.resolveProjectTarget(stringArg(args, "project"))
		if err != nil {
			return nil, err
		}
		items, err := a.listProjectFeatures(target.project, target.region)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"action":      "feature-list",
			"items":       items,
			"projectId":   target.project.ProjectID,
			"projectName": target.project.Name,
		}, nil

	case "agora.project.feature.status":
		feature := stringArg(args, "feature")
		if feature == "" {
			return nil, errors.New("feature is required")
		}
		if err := validateDoctorFeature(feature); err != nil {
			return nil, err
		}
		return a.projectFeatureStatus(feature, stringArg(args, "project"))

	case "agora.project.feature.enable":
		feature := stringArg(args, "feature")
		if feature == "" {
			return nil, errors.New("feature is required")
		}
		if err := validateDoctorFeature(feature); err != nil {
			return nil, err
		}
		return a.projectFeatureEnable(feature, stringArg(args, "project"))

	case "agora.project.webhook.events":
		return a.projectWebhookEvents(stringArg(args, "feature"))

	case "agora.project.webhook.list":
		return a.projectWebhookList(stringArg(args, "feature"), stringArg(args, "project"), false)

	case "agora.project.webhook.show":
		configID, err := configIDArg(args, "configId")
		if err != nil {
			return nil, err
		}
		return a.projectWebhookShow(
			configID,
			stringArg(args, "feature"),
			stringArg(args, "project"),
			boolArg(args, "withSecret", false),
		)

	case "agora.project.webhook.create":
		return a.projectWebhookCreate(webhookCreateOptions{
			Feature:        stringArg(args, "feature"),
			Project:        stringArg(args, "project"),
			URL:            stringArg(args, "url"),
			EventInputs:    stringSliceArg(args, "events"),
			Secret:         stringArg(args, "secret"),
			DeliveryRegion: stringArg(args, "deliveryRegion"),
		})

	case "agora.project.webhook.update":
		configID, err := configIDArg(args, "configId")
		if err != nil {
			return nil, err
		}
		return a.projectWebhookUpdate(webhookUpdateOptions{
			ConfigID:       configID,
			Feature:        stringArg(args, "feature"),
			Project:        stringArg(args, "project"),
			URL:            stringArg(args, "url"),
			EventInputs:    stringSliceArg(args, "events"),
			DeliveryRegion: stringArg(args, "deliveryRegion"),
			Enabled:        optionalBoolArg(args, "enabled"),
		})

	case "agora.project.webhook.delete":
		configID, err := configIDArg(args, "configId")
		if err != nil {
			return nil, err
		}
		if !boolArg(args, "confirm", false) {
			return nil, &cliError{Message: "confirmation required; pass confirm:true to delete this webhook configuration", Code: "CONFIRMATION_REQUIRED"}
		}
		return a.projectWebhookDelete(
			configID,
			stringArg(args, "feature"),
			stringArg(args, "project"),
		)

	case "agora.quickstart.list":
		items := []map[string]any{}
		for _, template := range quickstartTemplates() {
			if !template.Available {
				continue
			}
			items = append(items, map[string]any{"id": template.ID, "title": template.Title, "runtime": template.Runtime, "repoUrl": template.RepoURL, "supportsInit": template.SupportsInit})
		}
		return map[string]any{"items": items}, nil

	case "agora.quickstart.create":
		template, ok := findQuickstartTemplate(stringArg(args, "template"))
		if !ok {
			return nil, &cliError{Message: "unknown quickstart template. Run `agora quickstart list`.", Code: "QUICKSTART_TEMPLATE_UNKNOWN"}
		}
		target := defaultString(stringArg(args, "dir"), stringArg(args, "name"))
		if target == "" {
			return nil, errors.New("name or dir is required")
		}
		return a.quickstartCreate(*template, target, stringArg(args, "project"), stringArg(args, "ref"), progress)

	case "agora.quickstart.env_write":
		return a.quickstartEnvWrite(defaultString(stringArg(args, "dir"), "."), stringArg(args, "template"), stringArg(args, "project"))

	case "agora.init":
		name := stringArg(args, "name")
		if name == "" {
			return nil, &cliError{Message: "directory name is required", Code: "INIT_NAME_REQUIRED"}
		}
		template, ok := findQuickstartTemplate(stringArg(args, "template"))
		if !ok {
			return nil, &cliError{Message: "unknown quickstart template. Run `agora quickstart list`.", Code: "QUICKSTART_TEMPLATE_UNKNOWN"}
		}
		// CRITICAL: when serving over stdio, os.Stdin is the JSON-RPC
		// transport stream and os.Stderr might be observed by the
		// host. Pass an empty reader and an in-memory writer so a
		// future change to initProject can NEVER consume MCP frames or
		// scribble onto the host's stderr.
		var promptOut bytes.Buffer
		return a.initProject(
			name,
			defaultString(stringArg(args, "dir"), name),
			*template,
			stringArg(args, "project"),
			stringSliceArg(args, "features"),
			stringArg(args, "rtmDataCenter"),
			boolArg(args, "newProject", false),
			false,
			&promptOut,
			bytes.NewReader(nil),
			progress,
		)

	default:
		return nil, fmt.Errorf("unknown MCP tool %q", name)
	}
}

func stringArg(args map[string]any, key string) string {
	if value, ok := args[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func intArg(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return fallback
	}
}

func boolArg(args map[string]any, key string, fallback bool) bool {
	if value, ok := args[key].(bool); ok {
		return value
	}
	return fallback
}

func configIDArg(args map[string]any, key string) (int, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return 0, webhookConfigIDRequiredError()
	}
	switch v := value.(type) {
	case int:
		if v > 0 {
			return v, nil
		}
	case float64:
		maxInt := int(^uint(0) >> 1)
		if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) && math.Trunc(v) == v && v <= float64(maxInt) {
			return int(v), nil
		}
	}
	return 0, webhookConfigIDRequiredError()
}

func webhookConfigIDRequiredError() *cliError {
	return &cliError{Message: "webhook config ID is required", Code: "WEBHOOK_CONFIG_ID_REQUIRED"}
}

func optionalBoolArg(args map[string]any, key string) *bool {
	if value, ok := args[key].(bool); ok {
		return &value
	}
	return nil
}

// stringSliceArg coerces an "array of string" MCP argument into a Go
// slice. Accepts either a real []any payload (the JSON-RPC default) or
// a single comma-separated string for shells that flatten arrays.
func stringSliceArg(args map[string]any, key string) []string {
	value, ok := args[key]
	if !ok || value == nil {
		return nil
	}
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return v
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
