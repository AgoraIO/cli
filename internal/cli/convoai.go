package cli

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/AgoraIO/cli/internal/cli/playground"
)

type playgroundOptions struct {
	channel  string
	port     int
	uid      int64
	agentUID int64
	ttl      string
	noOpen   bool
}

func (a *App) buildConvoaiCommand() *cobra.Command {
	group := &cobra.Command{
		Use:   "convoai",
		Short: "Conversational AI developer tools",
		Long:  "Commands for building and testing Agora Conversational AI agents.",
	}
	group.AddCommand(a.buildConvoaiPlaygroundCommand())
	return group
}

func (a *App) buildConvoaiPlaygroundCommand() *cobra.Command {
	opts := &playgroundOptions{}
	cmd := &cobra.Command{
		Use:   "playground",
		Short: "Start a local frontend to talk to your Conversational AI agent",
		Long: `Serve a local web frontend that joins an RTC channel so you can talk to a
Conversational AI agent you start separately (e.g. a Python script on the
Agora Agents SDK). Point your agent at the same App ID and --channel.`,
		Example: example(`
  agora convoai playground --channel my-dev-room
  agora convoai playground --channel my-dev-room --port 8787 --no-open
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runConvoaiPlayground(cmd, opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.channel, "channel", "", "RTC channel to join (required; use the same value for your agent)")
	f.IntVar(&opts.port, "port", 8787, "local server port (auto-increments if busy unless explicitly set)")
	f.Int64Var(&opts.uid, "uid", 0, "browser RTC uid (0 = generate a non-zero uid)")
	f.Int64Var(&opts.agentUID, "agent-uid", 0, "reserved agent uid printed in the wire block (0 = generate)")
	f.StringVar(&opts.ttl, "ttl", "1h", "token lifetime")
	f.BoolVar(&opts.noOpen, "no-open", false, "do not open the browser automatically")
	_ = cmd.MarkFlagRequired("channel")
	return cmd
}

// Agora channel-name rules: 1–64 bytes, ASCII printable minus space and a
// small reserved set. Mirrors the documented allowed set.
const channelAllowedPunct = "!#$%&()+-:;<=.>?@[]^_{}|~,"

func validateChannelName(name string) error {
	if name == "" {
		return &cliError{Message: "--channel is required and must not be empty", Code: "CHANNEL_INVALID"}
	}
	if len(name) > 64 {
		return &cliError{Message: "--channel must be at most 64 bytes", Code: "CHANNEL_INVALID"}
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			strings.ContainsRune(channelAllowedPunct, r)
		if !ok {
			return &cliError{Message: fmt.Sprintf("--channel contains an unsupported character: %q", r), Code: "CHANNEL_INVALID"}
		}
	}
	return nil
}

func randInt(min, max int64) int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		panic("convoai: crypto/rand failed: " + err.Error())
	}
	return min + n.Int64()
}

func resolveUID(explicit int64) uint32 {
	if explicit > 0 {
		return uint32(explicit)
	}
	return uint32(randInt(1000, 9999999))
}

func resolveAgentUID(explicit int64) uint32 {
	if explicit > 0 {
		return uint32(explicit)
	}
	return uint32(randInt(10000000, 99999999))
}

func parseTTLSeconds(ttl string) (uint32, error) {
	d, err := time.ParseDuration(ttl)
	if err != nil {
		return 0, &cliError{Message: fmt.Sprintf("invalid --ttl %q: %v", ttl, err), Code: "TTL_INVALID"}
	}
	if d <= 0 {
		return 0, &cliError{Message: "--ttl must be positive", Code: "TTL_INVALID"}
	}
	return uint32(d.Seconds()), nil
}

type playgroundSession struct {
	appID        string
	appCert      string
	channel      string
	uid          uint32
	agentUID     uint32
	ttl          uint32 // seconds
	port         int
	explicitPort bool
	noOpen       bool
}

func featureEnabled(features []featureItem, id string) bool {
	for _, item := range features {
		if item.Feature == id {
			return item.Status == "enabled" || item.Status == "included"
		}
	}
	return false
}

func (a *App) resolvePlaygroundSession(cmd *cobra.Command, opts *playgroundOptions) (*playgroundSession, error) {
	if err := validateChannelName(opts.channel); err != nil {
		return nil, err
	}
	ttl, err := parseTTLSeconds(opts.ttl)
	if err != nil {
		return nil, err
	}
	target, err := a.resolveProjectTarget("")
	if err != nil {
		return nil, err
	}
	if target.project.SignKey == nil || *target.project.SignKey == "" {
		return nil, &cliError{
			Message: fmt.Sprintf("project %q does not have an app certificate. Enable one in Agora Console or use a different project with `agora project use`.", target.project.Name),
			Code:    "PROJECT_NO_CERTIFICATE",
		}
	}
	features, err := a.listProjectFeatures(target.project, target.region)
	if err != nil {
		return nil, err
	}
	if !featureEnabled(features, "convoai") {
		return nil, &cliError{
			Message: "project is not Conversational-AI ready. Run `agora project feature enable convoai` first.",
			Code:    "CONVOAI_NOT_READY",
		}
	}
	return &playgroundSession{
		appID:        target.project.AppID,
		appCert:      *target.project.SignKey,
		channel:      opts.channel,
		uid:          resolveUID(opts.uid),
		agentUID:     resolveAgentUID(opts.agentUID),
		ttl:          ttl,
		port:         opts.port,
		explicitPort: cmd.Flags().Changed("port"),
		noOpen:       opts.noOpen,
	}, nil
}

func (a *App) runConvoaiPlayground(cmd *cobra.Command, opts *playgroundOptions) error {
	sess, err := a.resolvePlaygroundSession(cmd, opts)
	if err != nil {
		return err
	}
	assets, err := playground.Assets()
	if err != nil {
		return err
	}
	ln, err := listenPlayground(sess.port, sess.explicitPort)
	if err != nil {
		return err
	}
	sess.port = ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", sess.port)

	srv := &http.Server{
		Handler:           newPlaygroundHandler(sess, assets),
		ReadHeaderTimeout: 10 * time.Second,
	}

	a.printPlaygroundStartup(cmd, sess, url)
	if !sess.noOpen && a.cfg.BrowserAutoOpen {
		_ = openBrowser(url)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// printPlaygroundStartup is fully implemented in Task 7. Minimal temporary
// version so this task compiles and running the command shows the URL.
func (a *App) printPlaygroundStartup(cmd *cobra.Command, sess *playgroundSession, url string) {
	fmt.Fprintf(cmd.OutOrStdout(), "Agora Convoai Playground running at %s (Ctrl-C to stop)\n", url)
}
