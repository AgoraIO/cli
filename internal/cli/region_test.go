package cli

import "testing"

func TestNormalizeLoginRegion(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to global", input: "", want: regionGlobal},
		{name: "global stays global", input: "global", want: regionGlobal},
		{name: "cn stays cn", input: "cn", want: regionCN},
		{name: "trim and lowercase", input: " CN ", want: regionCN},
		{name: "invalid rejected", input: "test", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeLoginRegion(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeLoginRegion(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCurrentRegionFromContext(t *testing.T) {
	cases := []struct {
		name  string
		input projectContext
		want  string
	}{
		{name: "empty defaults to global", input: projectContext{}, want: regionGlobal},
		{name: "global stays global", input: projectContext{CurrentRegion: regionGlobal}, want: regionGlobal},
		{name: "cn stays cn", input: projectContext{CurrentRegion: regionCN}, want: regionCN},
		{name: "case insensitive cn", input: projectContext{CurrentRegion: "CN"}, want: regionCN},
		{name: "invalid falls back to global", input: projectContext{CurrentRegion: "test"}, want: regionGlobal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := currentRegionFromContext(tc.input); got != tc.want {
				t.Fatalf("currentRegionFromContext(%+v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAuthRegionFromContextDefaultsToGlobal(t *testing.T) {
	app := &App{env: map[string]string{"XDG_CONFIG_HOME": t.TempDir()}}
	if got := app.authRegionFromContext(); got != regionGlobal {
		t.Fatalf("authRegionFromContext() = %q, want %q", got, regionGlobal)
	}
}

func TestAuthRegionReadsPersistedContext(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	if err := saveContext(env, projectContext{CurrentRegion: regionCN}); err != nil {
		t.Fatal(err)
	}
	app := &App{env: env}
	if got := app.authRegion(); got != regionCN {
		t.Fatalf("authRegion() = %q, want %q", got, regionCN)
	}
	if got := app.authRegionFromContext(); got != regionCN {
		t.Fatalf("authRegionFromContext() = %q, want %q", got, regionCN)
	}
}
