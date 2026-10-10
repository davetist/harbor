package main

import (
	"harbor/internal/config"
	"harbor/internal/server"
	"harbor/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestEphemeralListenerValidatesAssignedHostPort(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0", "localhost"} {
		t.Run(host, func(t *testing.T) {
			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			cfg.ListenAddress = host + ":0"
			listener, err := configuredListener(&cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			st, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			handler := server.New(cfg, st, nil)
			url := "http://" + cfg.ListenAddress + "/api/v1/ships"
			r := httptest.NewRequest("GET", url, nil)
			r.RemoteAddr = "127.0.0.1:9999"
			if host == "0.0.0.0" {
				r.Host = "127.0.0.1:" + cfg.ListenAddress[len(host)+1:]
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatal("assigned port rejected", w.Code)
			}
			r.Host = "127.0.0.1:0"
			w = httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatal("configured zero port accepted", w.Code)
			}
		})
	}
}
