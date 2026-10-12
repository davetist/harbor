package updates

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func publishedRelease(tag string, prerelease bool) map[string]any {
	return map[string]any{
		"tag_name": tag, "draft": false, "prerelease": prerelease,
		"published_at": "2026-10-12T00:00:00Z", "html_url": "https://untrusted.invalid/release",
		"assets": []map[string]any{
			{"name": "Harbor-" + strings.TrimPrefix(tag, "v") + "-macos-universal.dmg", "state": "uploaded", "size": 100},
			{"name": "SHA256SUMS", "state": "uploaded", "size": 100},
		},
	}
}

func fixtureChecker(t *testing.T, version string, serve http.HandlerFunc) *Checker {
	t.Helper()
	fixture := httptest.NewServer(serve)
	t.Cleanup(fixture.Close)
	checker := New(version)
	checker.endpoint = fixture.URL
	checker.platform = "darwin/arm64"
	return checker
}

func TestCheckFindsNewerPublishedInstaller(t *testing.T) {
	checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("User-Agent") != "Harbor" {
			t.Error("release request is missing public API headers")
		}
		// Deliberately unsorted: numeric identifiers must compare numerically.
		json.NewEncoder(w).Encode([]any{publishedRelease("v0.1.0-main.9", true), publishedRelease("v0.1.0-main.10", true), publishedRelease("v0.1.0-main.2", true)})
	})
	status := checker.Check(context.Background(), false)
	if !status.Available || !status.ComparisonKnown || !status.Prerelease || status.CurrentVersion != "0.1.0-main.9" || status.LatestVersion != "0.1.0-main.10" || status.Error != "" || status.CheckedAt.IsZero() {
		t.Fatalf("new prerelease was not offered: %+v", status)
	}
	if status.ReleaseURL != "https://github.com/davetist/harbor/releases/tag/v0.1.0-main.10" {
		t.Fatalf("release link did not stay on Harbor's public repository: %s", status.ReleaseURL)
	}
}

func TestCheckVersionAndReleaseEligibility(t *testing.T) {
	for _, trial := range []struct {
		name, current, latest        string
		prerelease, known, available bool
	}{
		{"equal", "0.1.0-main.10", "0.1.0-main.10", true, true, false},
		{"older release", "0.1.0-main.10", "0.1.0-main.9", true, true, false},
		{"stable after prerelease", "0.1.0-main.10", "0.1.0", false, true, true},
		{"stable ignores preview", "0.1.0", "0.2.0-main.1", true, true, false},
		{"stable update", "0.1.0", "0.2.0", false, true, true},
		{"dev is not an outdated release", "0.1.0-dev", "0.1.0-main.10", true, false, false},
		{"custom dev", "0.1.0-dev-20261010-adaptive", "0.1.0-main.10", true, false, false},
		{"unknown", "custom-build", "0.1.0-main.10", true, false, false},
		{"metadata", "0.1.0-main.10+local", "0.1.0-main.10", true, true, false},
	} {
		t.Run(trial.name, func(t *testing.T) {
			checker := fixtureChecker(t, trial.current, func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode([]any{publishedRelease("v"+trial.latest, trial.prerelease)})
			})
			status := checker.Check(context.Background(), false)
			if status.ComparisonKnown != trial.known || status.Available != trial.available || status.Error != "" {
				t.Fatalf("incorrect release comparison: %+v", status)
			}
			if trial.known && !trial.prerelease && status.LatestVersion != trial.latest {
				t.Fatalf("published stable version missing: %+v", status)
			}
		})
	}
	for _, name := range []string{"draft", "invalid version", "no installers", "unfinished upload", "missing checksums", "unpublished"} {
		t.Run(name, func(t *testing.T) {
			bad := publishedRelease("v0.2.0-main.99", true)
			switch name {
			case "draft":
				bad["draft"] = true
			case "invalid version":
				bad["tag_name"] = "v0.2.0-main.099"
			case "no installers":
				bad["assets"] = []any{}
			case "unfinished upload":
				bad["assets"].([]map[string]any)[0]["state"] = "new"
			case "missing checksums":
				bad["assets"] = bad["assets"].([]map[string]any)[:1]
			case "unpublished":
				bad["published_at"] = nil
			}
			checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode([]any{bad, publishedRelease("v0.1.0-main.10", true)})
			})
			status := checker.Check(context.Background(), false)
			if status.LatestVersion != "0.1.0-main.10" || !status.Available {
				t.Fatalf("incomplete release displaced an installable release: %+v", status)
			}
		})
	}
}

func TestCheckCachesAndManualCheckRefreshes(t *testing.T) {
	var requests atomic.Int32
	checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) {
		tag := "v0.1.0-main.9"
		if requests.Add(1) > 1 {
			tag = "v0.1.0-main.10"
		}
		json.NewEncoder(w).Encode([]any{publishedRelease(tag, true)})
	})
	now := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	checker.now = func() time.Time { return now }
	if checker.Check(context.Background(), false).Available {
		t.Fatal("current release was offered as an update")
	}
	now = now.Add(time.Minute)
	if checker.Check(context.Background(), false).Available || requests.Load() != 1 {
		t.Fatal("background check did not use its cache")
	}
	if !checker.Check(context.Background(), true).Available || requests.Load() != 2 {
		t.Fatal("manual check did not discover the newly published update")
	}
	now = now.Add(6*time.Hour + time.Second)
	if !checker.Check(context.Background(), false).Available || requests.Load() != 3 {
		t.Fatal("expired background cache was not refreshed")
	}
}

func TestCheckOfflineKeepsKnownUpdateAndReportsFailure(t *testing.T) {
	var offline atomic.Bool
	var requests atomic.Int32
	checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if offline.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		json.NewEncoder(w).Encode([]any{publishedRelease("v0.1.0-main.10", true)})
	})
	before := checker.Check(context.Background(), false)
	offline.Store(true)
	after := checker.Check(context.Background(), true)
	if after.Error == "" || !after.Available || after.LatestVersion != before.LatestVersion || after.CheckedAt != before.CheckedAt {
		t.Fatalf("failed check discarded a known update: %+v", after)
	}
	checker.Check(context.Background(), false)
	if requests.Load() != 2 {
		t.Fatal("offline background calls repeatedly hit GitHub")
	}
}

func TestCheckRejectsMalformedResponseWithoutClaimingUpToDate(t *testing.T) {
	for _, data := range []string{"<html>unavailable</html>", `{"message":"rate limited"}`, `[] trailing`, strings.Repeat(" ", 4<<20+1)} {
		checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(data)) })
		status := checker.Check(context.Background(), false)
		if status.Error == "" || !status.CheckedAt.IsZero() || status.LatestVersion != "" || status.Available {
			t.Fatalf("invalid response fabricated a successful check: %+v", status)
		}
	}
}

func TestCheckOnlyOffersInstallersForTheServiceHost(t *testing.T) {
	checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]any{publishedRelease("v0.1.0-main.10", true)})
	})
	checker.platform = "linux/arm64"
	status := checker.Check(context.Background(), false)
	if status.Available || status.LatestVersion != "" {
		t.Fatalf("Mac-only release was offered on Linux: %+v", status)
	}
}

func TestConcurrentBackgroundChecksShareOneReleaseRequest(t *testing.T) {
	var requests atomic.Int32
	checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode([]any{publishedRelease("v0.1.0-main.10", true)})
	})
	var group sync.WaitGroup
	for range 10 {
		group.Go(func() {
			status := checker.Check(context.Background(), false)
			if !status.Available || status.LatestVersion != "0.1.0-main.10" {
				t.Errorf("concurrent reader missed the cached update: %+v", status)
			}
		})
	}
	group.Wait()
	if requests.Load() != 1 {
		t.Fatalf("shared background checks made %d release requests", requests.Load())
	}
}

func TestCheckFindsMatchingLinuxInstaller(t *testing.T) {
	for _, platform := range []string{"linux/amd64", "linux/arm64"} {
		t.Run(platform, func(t *testing.T) {
			checker := fixtureChecker(t, "0.1.0-main.9", func(w http.ResponseWriter, r *http.Request) {
				release := publishedRelease("v0.1.0-main.10", true)
				release["assets"].([]map[string]any)[0]["name"] = "harbor-0.1.0-main.10-" + strings.ReplaceAll(platform, "/", "-") + ".run"
				json.NewEncoder(w).Encode([]any{release})
			})
			checker.platform = platform
			status := checker.Check(context.Background(), false)
			if !status.Available || status.LatestVersion != "0.1.0-main.10" {
				t.Fatalf("matching Linux installer missing: %+v", status)
			}
		})
	}
}
