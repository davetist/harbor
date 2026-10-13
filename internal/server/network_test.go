package server

import (
	"context"
	"encoding/json"
	"fmt"
	"harbor/internal/model"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func networkRequest(method, host, local, remote, path string) *http.Request {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(`{}`))
	r.RemoteAddr = net.JoinHostPort(remote, "12345")
	return r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP(local), Port: 18789}))
}

func TestNetworkHostsAndOrigins(t *testing.T) {
	h := handler(t)
	h.cfg.ListenAddress = "0.0.0.0:18789"
	for _, tc := range []struct {
		name, host, local, remote, origin string
		allowed                           bool
	}{
		{"LAN", "192.168.50.10:18789", "192.168.50.10", "192.168.50.20", "http://192.168.50.10:18789", true},
		{"Tailscale", "100.64.0.10:18789", "100.64.0.10", "100.64.0.20", "http://100.64.0.10:18789", true},
		{"localhost", "localhost:18789", "127.0.0.1", "127.0.0.1", "http://localhost:18789", true},
		{"link local", "169.254.1.2:18789", "169.254.1.2", "169.254.1.3", "", true},
		{"peer outside private ranges", "192.168.50.10:18789", "192.168.50.10", "198.51.100.20", "", true},
		{"interface outside private ranges", "192.0.2.10:18789", "192.0.2.10", "192.168.50.20", "http://192.0.2.10:18789", true},
		{"below shared range", "100.63.1.2:18789", "100.63.1.2", "100.63.1.3", "", true},
		{"above shared range", "100.128.1.2:18789", "100.128.1.2", "100.128.1.3", "", true},
		{"spoofed private host", "192.168.50.99:18789", "192.168.50.10", "192.168.50.20", "", false},
		{"DNS rebinding", "evil.invalid:18789", "192.168.50.10", "192.168.50.20", "", false},
		{"wrong port", "192.168.50.10:8789", "192.168.50.10", "192.168.50.20", "", false},
		{"cross origin", "192.168.50.10:18789", "192.168.50.10", "192.168.50.20", "http://evil.invalid:18789", false},
		{"spoofed host outside private ranges", "192.0.2.11:18789", "192.0.2.10", "198.51.100.20", "", false},
		{"cross origin outside private ranges", "192.0.2.10:18789", "192.0.2.10", "198.51.100.20", "http://evil.invalid:18789", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := networkRequest("POST", tc.host, tc.local, tc.remote, "/api/v1/ships/a/start")
			r.Header.Set("Origin", tc.origin)
			if got := h.allowed(r); got != tc.allowed {
				t.Fatalf("allowed=%v want %v", got, tc.allowed)
			}
		})
	}
	r := networkRequest("POST", "192.168.50.10:18789", "192.168.50.10", "192.168.50.20", "/api/v1/ships/a/start")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if h.allowed(r) {
		t.Fatal("cross-site mutation accepted")
	}
}

func TestIPv6WildcardUsesDestinationHost(t *testing.T) {
	h := handler(t)
	h.cfg.ListenAddress = "[::]:18789"
	r := networkRequest("POST", "[2001:db8::10]:18789", "2001:db8::10", "2001:db8::20", "/api/v1/ships/a/start")
	r.Header.Set("Origin", "http://[2001:db8::10]:18789")
	if !h.allowed(r) {
		t.Fatal("matching IPv6 host and origin rejected")
	}
	r.Host = "[2001:db8::11]:18789"
	r.Header.Set("Origin", "http://[2001:db8::11]:18789")
	if h.allowed(r) {
		t.Fatal("spoofed IPv6 host accepted")
	}
}

func TestOpenShipUsesBrowserHost(t *testing.T) {
	h := handler(t)
	h.cfg.ListenAddress = "0.0.0.0:18789"
	h.cfg.AdvertisedHost = "ship.example"
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ready")) }))
	defer web.Close()
	port := web.Listener.Addr().(*net.TCPAddr).Port
	if err := h.store.PutShip(context.Background(), model.Ship{ID: "a", PierPath: "/test/pier", State: "running", HTTPBindAddress: "127.0.0.1", WebPort: port}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host, local, want string
	}{
		{"127.0.0.1", "127.0.0.1", "http://127.0.0.1:%d"},
		{"localhost", "127.0.0.1", "http://localhost:%d"},
		{"192.168.50.10", "192.168.50.10", "http://192.168.50.10:%d"},
		{"100.64.0.10", "100.64.0.10", "http://100.64.0.10:%d"},
		{"ship.example", "100.64.0.10", "http://ship.example:%d"},
		{"192.0.2.10", "192.0.2.10", "http://192.0.2.10:%d"},
		{"100.63.1.2", "100.63.1.2", "http://100.63.1.2:%d"},
		{"100.128.1.2", "100.128.1.2", "http://100.128.1.2:%d"},
		{"2001:db8::10", "2001:db8::10", "http://[2001:db8::10]:%d"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			r := networkRequest("GET", net.JoinHostPort(tc.host, "18789"), tc.local, tc.local, "/api/v1/ships/a/open")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			var result struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.URL != fmt.Sprintf(tc.want, port) {
				t.Fatalf("%s: %d %s (%v)", tc.host, w.Code, w.Body.String(), err)
			}
		})
	}
	r := networkRequest("GET", "other.example:18789", "100.64.0.10", "100.64.0.20", "/api/v1/ships/a/open")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unconfigured hostname accepted: %d %s", w.Code, w.Body.String())
	}
}
