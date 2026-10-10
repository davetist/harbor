package server

import (
	"crypto/rand"
	"encoding/base64"
	"harbor/internal/ui"
	"io/fs"
	"net/http"
	"strings"
)

func contentPolicy(nonce string) string {
	styles := "'self'"
	if nonce != "" {
		styles += " 'nonce-" + nonce + "'"
	}
	return "default-src 'self'; script-src 'self'; style-src " + styles + "; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'"
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	b, e := fs.ReadFile(ui.Files(), "index.html")
	if e != nil {
		fail(w, 503, "assets", "The embedded interface is unavailable.")
		return
	}
	var random [24]byte
	if _, e = rand.Read(random[:]); e != nil {
		fail(w, 503, "random", "Unable to prepare interface policy.")
		return
	}
	nonce := base64.RawStdEncoding.EncodeToString(random[:])
	page := strings.Replace(string(b), "</head>", `<meta name="harbor-style-nonce" content="`+nonce+`"></head>`, 1)
	w.Header().Set("Content-Security-Policy", contentPolicy(nonce))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}
