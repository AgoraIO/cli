package cli

import (
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/AgoraIO/cli/internal/agoratoken"
)

type playgroundHandler struct {
	sess   *playgroundSession
	static http.Handler // serves embedded assets; nil in unit tests
}

func newPlaygroundHandler(sess *playgroundSession, assets fs.FS) http.Handler {
	h := &playgroundHandler{sess: sess}
	if assets != nil {
		h.static = http.FileServer(http.FS(assets))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/get_config", h.handleGetConfig)
	mux.HandleFunc("/", h.handleStatic)
	return loopbackGuard(mux)
}

// loopbackGuard rejects requests whose Host is not loopback (defeats DNS-rebinding).
func loopbackGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *playgroundHandler) handleStatic(w http.ResponseWriter, r *http.Request) {
	if h.static == nil {
		http.NotFound(w, r)
		return
	}
	h.static.ServeHTTP(w, r)
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
