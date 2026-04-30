package cli

import (
	"fmt"
	"strings"
)

// featureCatalog is the single source of truth for the supported
// product features the CLI knows how to enable, inspect, and report on.
// Adding a new product feature requires only a new entry here:
//
//   - command-line completions (`completeFeatureIDs`),
//   - the `init`/`project create` default-features list,
//   - `project doctor`'s feature validator,
//   - `project feature {list,status,enable}`'s iteration order, and
//   - the MCP tool surface
//
// all read from this slice. Order is significant: it controls the
// stable ordering of `project feature list` output, the deterministic
// ordering of `project create --feature` defaults, and the suggestion
// order in shell completions.
var featureCatalog = []featureSpec{
	{ID: "rtc", Title: "Real-Time Communication"},
	{ID: "rtm", Title: "Real-Time Messaging"},
	{ID: "convoai", Title: "Conversational AI"},
}

// featureSpec describes one product feature the CLI knows about.
// Future fields (Beta, GA timestamp, MCP exposure flag, etc.) belong
// here so the catalog stays the single source of truth.
type featureSpec struct {
	ID    string
	Title string
}

// featureIDs returns the canonical list of feature IDs in the
// catalog's stable order. Callers must NOT mutate the returned slice;
// it is freshly allocated on every call so accidental aliasing is
// safe.
func featureIDs() []string {
	out := make([]string, 0, len(featureCatalog))
	for _, f := range featureCatalog {
		out = append(out, f.ID)
	}
	return out
}

// isKnownFeature reports whether id is one of the catalog's known
// feature IDs.
func isKnownFeature(id string) bool {
	id = strings.TrimSpace(id)
	for _, f := range featureCatalog {
		if f.ID == id {
			return true
		}
	}
	return false
}

// featureListString is the human-readable representation of the
// catalog used in error messages (e.g. "rtc, rtm, convoai"). Keeping
// it derived from the catalog means error messages stay correct as
// new features are added.
func featureListString() string {
	return strings.Join(featureIDs(), ", ")
}

// validateFeatureID returns a stable error matching the historical
// `project doctor --feature` shape. The wording is intentionally
// preserved so existing scripted error parsers do not break.
func validateFeatureID(id string) error {
	if isKnownFeature(id) {
		return nil
	}
	return fmt.Errorf("%q must be one of: %s.", id, featureListString())
}
