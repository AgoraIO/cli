package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const projectCompletionPageSize = 100

// completeProjectNames is the dynamic completion for `agora project use`,
// `agora project show`, and the project arg of `agora project feature`.
//
// It is cache-first *only when a local session exists*: a fresh on-disk
// cache (under <AGORA_HOME>/cache/projects.json) is served instantly so
// the shell never blocks on the network for a TAB. With no session file
// (or empty token), the cache is ignored so Tab never suggests stale
// projects after logout. When the cache is missing or stale, we fall back
// to a single live API call and warm the cache for next time. If the
// network call also fails (e.g. the user is unauthenticated), we
// silently return no completions — never a partial list and never an
// auth-flow side effect, since completion runs from inside the user's
// shell on every keystroke.
func (a *App) completeProjectNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if items, ok := a.completionProjectsFromCache(); ok {
		return filterProjectCompletions(items, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	list, err := a.listProjects("", 1, projectCompletionPageSize)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return filterProjectCompletions(list.Items, toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completionProjectsFromCache returns the cached project items when
// the user has a persisted non-empty session, the on-disk cache exists,
// parses, and is younger than the current process's TTL (overridable
// via AGORA_PROJECT_CACHE_TTL_SECONDS).
func (a *App) completionProjectsFromCache() ([]projectSummary, bool) {
	if !hasPersistedNonEmptySession(a.env) {
		return nil, false
	}
	payload, fresh, err := loadProjectListCache(a.env)
	if err != nil || !fresh {
		return nil, false
	}
	if cacheTTLFromEnv(a.env) == 0 {
		return nil, false
	}
	if payload.FetchedAt == "" {
		return nil, false
	}
	if fetchedAt, parseErr := time.Parse(time.RFC3339Nano, payload.FetchedAt); parseErr == nil {
		if time.Since(fetchedAt) > cacheTTLFromEnv(a.env) {
			return nil, false
		}
	}
	return payload.Items, true
}

// filterProjectCompletions emits both name- and ID-prefixed matches
// (with the alternate value as the description), sorted by their
// natural appearance in the cache.
func filterProjectCompletions(items []projectSummary, toComplete string) []string {
	results := make([]string, 0, len(items)*2)
	prefix := strings.ToLower(toComplete)
	for _, item := range items {
		if item.Name != "" && strings.HasPrefix(strings.ToLower(item.Name), prefix) {
			results = append(results, fmt.Sprintf("%s\t%s", item.Name, item.ProjectID))
		}
		if item.ProjectID != "" && strings.HasPrefix(strings.ToLower(item.ProjectID), prefix) {
			results = append(results, fmt.Sprintf("%s\t%s", item.ProjectID, item.Name))
		}
	}
	return results
}

func completeQuickstartTemplateIDs(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	results := []string{}
	for _, template := range quickstartTemplates() {
		if !template.Available {
			continue
		}
		if strings.HasPrefix(strings.ToLower(template.ID), strings.ToLower(toComplete)) {
			results = append(results, fmt.Sprintf("%s\t%s", template.ID, template.Title))
		}
	}
	return results, cobra.ShellCompDirectiveNoFileComp
}

func completeFeatureIDs(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	results := []string{}
	for _, feature := range featureIDs() {
		if strings.HasPrefix(strings.ToLower(feature), strings.ToLower(toComplete)) {
			results = append(results, feature)
		}
	}
	return results, cobra.ShellCompDirectiveNoFileComp
}

func (a *App) completeFeatureThenProject(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completeFeatureIDs(cmd, args, toComplete)
	}
	if len(args) == 1 {
		return a.completeProjectNames(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
