package cli

import (
	"fmt"
	"strings"
)

const (
	regionGlobal = "global"
	regionCN     = "cn"
)

// normalizeLoginRegion validates the login flag value and resolves the
// effective control-plane region used for login.
func normalizeLoginRegion(region string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(region)) {
	case "", regionGlobal:
		return regionGlobal, nil
	case regionCN:
		return regionCN, nil
	default:
		return "", fmt.Errorf("--region must be one of: %s, %s", regionGlobal, regionCN)
	}
}

// normalizeContextRegion canonicalizes a persisted context region value and
// falls back to the global control plane for empty or unknown inputs.
func normalizeContextRegion(region string) string {
	if strings.EqualFold(strings.TrimSpace(region), regionCN) {
		return regionCN
	}
	return regionGlobal
}

// currentRegionFromContext returns the canonical active region stored in the
// persisted project context.
func currentRegionFromContext(ctx projectContext) string {
	return normalizeContextRegion(ctx.CurrentRegion)
}

// authRegion returns the current control-plane region for authenticated CLI
// operations, defaulting to global when context loading fails.
func (a *App) authRegion() string {
	ctx, err := loadContext(a.env)
	if err != nil {
		return regionGlobal
	}
	return currentRegionFromContext(ctx)
}

// authRegionFromContext returns the current control-plane region from the
// persisted context and falls back to global when the context is unavailable.
func (a *App) authRegionFromContext() string {
	ctx, err := loadContext(a.env)
	if err != nil {
		return regionGlobal
	}
	return currentRegionFromContext(ctx)
}
