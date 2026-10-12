package server

import (
	"context"
	"encoding/json"
	"harbor/internal/updates"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHarborVersionAndUpdateRoutes(t *testing.T) {
	h := handler(t)
	h.SetVersion("0.1.0-main.9")
	var manual []bool
	h.updateCheck = func(ctx context.Context, force bool) updates.Status {
		manual = append(manual, force)
		return updates.Status{CurrentVersion: "0.1.0-main.9", LatestVersion: "0.1.0-main.10", Available: true, ComparisonKnown: true, Prerelease: true, ReleaseURL: "https://github.com/davetist/harbor/releases/tag/v0.1.0-main.10"}
	}
	for _, path := range []string{"/host", "/updates", "/updates/check"} {
		method := "GET"
		if path == "/updates/check" {
			method = "POST"
		}
		r := httptest.NewRequest(method, "http://127.0.0.1:8789/api/v1"+path, strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if path == "/host" {
			if body["harborVersion"] != "0.1.0-main.9" {
				t.Fatalf("footer build version missing: %v", body)
			}
		} else if body["available"] != true || body["latestVersion"] != "0.1.0-main.10" {
			t.Fatalf("update metadata missing: %v", body)
		}
	}
	if len(manual) != 2 || manual[0] || !manual[1] {
		t.Fatalf("manual route did not refresh the release cache: %v", manual)
	}
}

func TestHarborUpdateCheckPreservesOriginAndInputGuards(t *testing.T) {
	h := handler(t)
	called := false
	h.updateCheck = func(ctx context.Context, force bool) updates.Status { called = true; return updates.Status{} }
	for _, trial := range []struct {
		origin, body string
		status       int
	}{
		{"https://untrusted.invalid", `{}`, 403},
		{"", `{"unexpected":true}`, 422},
		{"", `{} {}`, 422},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/updates/check", strings.NewReader(trial.body))
		r.Header.Set("Origin", trial.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != trial.status || called {
			t.Fatalf("invalid update request reached the release feed: %d", w.Code)
		}
	}
}
