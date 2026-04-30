package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (a *App) completeProjectNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	list, err := a.listProjects(toComplete, 1, 100)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	results := make([]string, 0, len(list.Items)*2)
	for _, item := range list.Items {
		if item.Name != "" && strings.HasPrefix(strings.ToLower(item.Name), strings.ToLower(toComplete)) {
			results = append(results, fmt.Sprintf("%s\t%s", item.Name, item.ProjectID))
		}
		if item.ProjectID != "" && strings.HasPrefix(strings.ToLower(item.ProjectID), strings.ToLower(toComplete)) {
			results = append(results, fmt.Sprintf("%s\t%s", item.ProjectID, item.Name))
		}
	}
	return results, cobra.ShellCompDirectiveNoFileComp
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
