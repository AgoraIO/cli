package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/AgoraIO/cli/internal/agoratoken"
)

type playgroundHandler struct {
	sess *playgroundSession
}

func newPlaygroundHandler(sess *playgroundSession, assets fs.FS) http.Handler {
	h := &playgroundHandler{sess: sess}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/get_config", h.handleGetConfig)
	mux.Handle("/", newStaticHandler(assets))
	return loopbackGuard(mux)
}

func newStaticHandler(assets fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if assets == nil {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || strings.HasSuffix(name, "/") {
			name += "index.html"
		}
		// exact file?
		if b, err := fs.ReadFile(assets, name); err == nil {
			http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(b))
			return
		}
		// gzipped sibling?
		if b, err := fs.ReadFile(assets, name+".gz"); err == nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			ct := mime.TypeByExtension(path.Ext(name))
			if ct == "" {
				ct = "application/octet-stream"
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(b)
			return
		}
		http.NotFound(w, r)
	}
}

// loopbackGuard rejects requests whose Host is not loopback (defeats DNS-rebinding).
func loopbackGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.TrimSuffix(host, ".")
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *playgroundHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	uid := h.sess.uid
	if q := r.URL.Query().Get("uid"); q != "" {
		if parsed, err := strconv.ParseUint(q, 10, 32); err == nil && parsed != 0 {
			uid = uint32(parsed)
		}
	}

	issue := uint32(time.Now().Unix())
	tk := agoratoken.NewAccessToken2(h.sess.appID, h.sess.appCert, issue, h.sess.ttl)
	privExpire := issue + h.sess.ttl
	tk.AddRtcService(h.sess.channel, uid, privExpire)
	tk.AddRtmService(strconv.FormatUint(uint64(uid), 10), privExpire)
	token, err := tk.Build()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 1, "msg": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"app_id":       h.sess.appID,
			"token":        token,
			"uid":          strconv.FormatUint(uint64(uid), 10),
			"channel_name": h.sess.channel,
			"agent_uid":    strconv.FormatUint(uint64(h.sess.agentUID), 10),
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	// Same-origin only: no Access-Control-Allow-Origin header is emitted.
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// listenPlayground binds 127.0.0.1:port. When explicit is false it tries the
// requested port and up to 20 following ports; when explicit is true a busy
// port is a hard error.
func listenPlayground(port int, explicit bool) (net.Listener, error) {
	max := port
	if !explicit {
		max = port + 20
	}
	for p := port; p <= max; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, nil
		}
		if explicit {
			return nil, fmt.Errorf("port %d is in use", p)
		}
	}
	return nil, fmt.Errorf("no free port found in %d-%d", port, max)
}
