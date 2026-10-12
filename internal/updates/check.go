// Package updates checks Harbor's public installer releases. It never replaces
// the running executable or changes ship state.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"
)

const releaseFeed = "https://api.github.com/repos/davetist/harbor/releases?per_page=100"
const checkFailure = "Could not check for updates. Try again later."

type Status struct {
	CurrentVersion  string    `json:"currentVersion"`
	LatestVersion   string    `json:"latestVersion"`
	ComparisonKnown bool      `json:"comparisonKnown"`
	Available       bool      `json:"available"`
	Prerelease      bool      `json:"prerelease"`
	ReleaseURL      string    `json:"releaseURL"`
	CheckedAt       time.Time `json:"checkedAt"`
	Error           string    `json:"error,omitempty"`
}

type Checker struct {
	mu          sync.Mutex
	status      Status
	nextAttempt time.Time
	endpoint    string
	platform    string
	client      *http.Client
	now         func() time.Time
}

func New(version string) *Checker {
	version = strings.TrimPrefix(version, "v")
	current, valid := parseVersion(version)
	known := valid && !current.development()
	return &Checker{
		status:   Status{CurrentVersion: version, ComparisonKnown: known},
		endpoint: releaseFeed, platform: runtime.GOOS + "/" + runtime.GOARCH,
		client: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}
}

// Background checks share a six-hour cache across browser sessions. Manual
// checks refresh it; failed background checks wait a minute before retrying.
func (c *Checker) Check(ctx context.Context, force bool) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if !force && now.Before(c.nextAttempt) {
		return c.status
	}
	releases, err := c.fetch(ctx)
	if err != nil {
		// Keep any previously discovered update visible when GitHub is offline.
		c.status.Error = checkFailure
		c.nextAttempt = now.Add(time.Minute)
		return c.status
	}
	current, _ := parseVersion(c.status.CurrentVersion)
	stable := c.status.ComparisonKnown && len(current.pre) == 0
	status := Status{CurrentVersion: c.status.CurrentVersion, ComparisonKnown: c.status.ComparisonKnown, CheckedAt: now}
	var latest version
	for _, release := range releases {
		candidate, valid := parseVersion(release.Tag)
		prerelease := release.Prerelease || len(candidate.pre) > 0
		if !valid || release.Draft || release.PublishedAt.IsZero() || (stable && prerelease) || !release.installable(c.platform) {
			continue
		}
		if status.LatestVersion != "" && candidate.compare(latest) <= 0 {
			continue
		}
		latest = candidate
		status.LatestVersion = strings.TrimPrefix(release.Tag, "v")
		status.Prerelease = prerelease
		status.ReleaseURL = "https://github.com/davetist/harbor/releases/tag/" + url.PathEscape(release.Tag)
		status.Available = status.ComparisonKnown && candidate.compare(current) > 0
	}
	c.status = status
	c.nextAttempt = now.Add(6 * time.Hour)
	return status
}

type release struct {
	Tag         string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name  string `json:"name"`
		State string `json:"state"`
		Size  int64  `json:"size"`
	} `json:"assets"`
}

func (r release) installable(platform string) bool {
	version := strings.TrimPrefix(r.Tag, "v")
	var installer string
	switch platform {
	case "darwin/arm64", "darwin/amd64":
		installer = "Harbor-" + version + "-macos-universal.dmg"
	case "linux/arm64", "linux/amd64":
		installer = "harbor-" + version + "-linux-" + strings.TrimPrefix(platform, "linux/") + ".run"
	default:
		return false
	}
	ready, checksums := false, false
	for _, asset := range r.Assets {
		if asset.State != "uploaded" || asset.Size <= 0 {
			continue
		}
		ready = ready || asset.Name == installer
		checksums = checksums || asset.Name == "SHA256SUMS"
	}
	return ready && checksums
}

func (c *Checker) fetch(ctx context.Context) ([]release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Harbor")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("release feed unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20+1))
	if err != nil || len(data) > 4<<20 {
		return nil, errors.New("release feed exceeds limit")
	}
	var releases []release
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, err
	}
	if releases == nil {
		return nil, errors.New("invalid release list")
	}
	return releases, nil
}
