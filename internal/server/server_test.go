package server

import (
	"context"
	"harbor/internal/config"
	"harbor/internal/model"
	"harbor/internal/store"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func handler(t *testing.T) *Server {
	t.Helper()
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	c, e := config.Load("")
	if e != nil {
		t.Fatal(e)
	}
	c.ListenAddress = "127.0.0.1:8789"
	return New(c, s, nil)
}
func TestCrossOriginMutationRejected(t *testing.T) {
	h := handler(t)
	r := httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/ships/a/start", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://evil.invalid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
func TestUnknownShipIs404(t *testing.T) {
	h := handler(t)
	r := httptest.NewRequest("GET", "http://127.0.0.1:8789/api/v1/ships/missing", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
func TestSlowEventClientCannotBlockPublisher(t *testing.T) {
	b := NewBroker()
	ch, cancel := b.Subscribe()
	defer cancel()
	for i := 0; i < 130; i++ {
		b.Publish(model.Event{Type: "ship"})
	}
	n := 0
	for range ch {
		n++
	}
	if n > 128 {
		t.Fatal(n)
	}
}
func TestProductionRegistryEmptyAndActionsUnavailable(t *testing.T) {
	h := handler(t)
	r := httptest.NewRequest("GET", "http://127.0.0.1:8789/api/v1/ships", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("fixture leaked: %s", w.Body.String())
	}
	h.store.PutShip(context.Background(), model.Ship{ID: "a", PierPath: "/test/pier"})
	r = httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/ships/a/start", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("unwired action fabricated success %d", w.Code)
	}
}
func TestHostSpoofingRejected(t *testing.T) {
	h := handler(t)
	r := httptest.NewRequest("GET", "http://evil.invalid:8789/api/v1/ships", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}

func TestHTMLNonceMatchesContentPolicy(t *testing.T) {
	h := handler(t)
	nonces := map[string]bool{}
	pattern := regexp.MustCompile(`<meta name="harbor-style-nonce" content="([^"]+)"`)
	for range 2 {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8789/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		m := pattern.FindStringSubmatch(w.Body.String())
		if len(m) != 2 {
			t.Fatal("HTML is missing terminal style nonce")
		}
		policy := w.Header().Get("Content-Security-Policy")
		if !strings.Contains(policy, "'nonce-"+m[1]+"'") || strings.Contains(policy, "unsafe-inline") {
			t.Fatal("nonce/CSP mismatch")
		}
		nonces[m[1]] = true
	}
	if len(nonces) != 2 {
		t.Fatal("nonce reused")
	}
}
