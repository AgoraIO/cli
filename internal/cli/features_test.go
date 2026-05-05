package cli

import (
	"strings"
	"testing"
)

func TestFeatureCatalogIsTheSingleSourceOfTruth(t *testing.T) {
	ids := featureIDs()
	if len(ids) == 0 {
		t.Fatal("featureIDs() returned an empty list; the catalog is the source of truth and must never be empty")
	}
	for _, id := range ids {
		if !isKnownFeature(id) {
			t.Errorf("isKnownFeature(%q) = false but %q is in featureIDs()", id, id)
		}
	}
	if isKnownFeature("not-a-feature") {
		t.Error("isKnownFeature should reject unknown ids")
	}
	expectedString := strings.Join(ids, ", ")
	if got := featureListString(); got != expectedString {
		t.Errorf("featureListString() = %q, want %q", got, expectedString)
	}
}

func TestFeatureIDsReturnsFreshSliceEachCall(t *testing.T) {
	a := featureIDs()
	b := featureIDs()
	if len(a) > 0 {
		a[0] = "MUTATED"
	}
	if b[0] == "MUTATED" {
		t.Fatal("featureIDs() must return a fresh slice; mutating one return value affected another")
	}
}

func TestValidateFeatureIDIncludesActualCatalog(t *testing.T) {
	for _, id := range featureIDs() {
		if err := validateFeatureID(id); err != nil {
			t.Errorf("validateFeatureID(%q) = %v, want nil", id, err)
		}
	}
	err := validateFeatureID("nope")
	if err == nil {
		t.Fatal("validateFeatureID(unknown) should return an error")
	}
	if !strings.Contains(err.Error(), featureListString()) {
		t.Errorf("error message should list the catalog, got %q", err.Error())
	}
}
