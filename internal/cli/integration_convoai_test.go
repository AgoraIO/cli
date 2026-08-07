package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AgoraIO/cli/internal/cli/playground"
)

func TestConvoaiPlaygroundServesEmbeddedApp(t *testing.T) {
	assets, err := playground.Assets()
	if err != nil {
		t.Fatal(err)
	}
	sess := &playgroundSession{
		appID:    "970CA35de60c44645bbae8a215061b33",
		appCert:  "5CFd2fd1755d40ecb72977518be15d3b",
		channel:  "smoke",
		uid:      12345,
		agentUID: 87654321,
		ttl:      3600,
	}
	srv := httptest.NewServer(newPlaygroundHandler(sess, assets))
	defer srv.Close()

	// Index page: httptest server binds 127.0.0.1, so the loopback guard passes.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("index not served: status=%d len=%d", resp.StatusCode, len(body))
	}
	// The real built SPA has a #root mount point; the placeholder had a
	// "not built" message. Assert we're serving the REAL app.
	if !strings.Contains(string(body), "root") {
		t.Fatalf("served index does not look like the built SPA: %.120q", body)
	}

	// Token endpoint returns a 007 token in the {code,data} envelope.
	tok, err := http.Get(srv.URL + "/api/get_config")
	if err != nil {
		t.Fatal(err)
	}
	tbody, _ := io.ReadAll(tok.Body)
	tok.Body.Close()
	if tok.StatusCode != http.StatusOK || !strings.Contains(string(tbody), "\"007") {
		t.Fatalf("get_config not served correctly: status=%d body=%.160q", tok.StatusCode, tbody)
	}
}
