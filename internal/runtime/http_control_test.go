package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"harbor/internal/model"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortHTTPCodeQueryChecksIdentityBoundsAndDoesNotFollowRedirects(t *testing.T) {
	for _, scenario := range []string{"success", "duplicate listener", "wrong identity", "oversize", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Source struct {
						Dojo string `json:"dojo"`
					} `json:"source"`
				}
				json.NewDecoder(r.Body).Decode(&request)
				if request.Source.Dojo == "our" {
					if scenario == "wrong identity" {
						fmt.Fprint(w, `"~nec"`)
					} else {
						fmt.Fprint(w, `"~zod\n"`)
					}
					return
				}
				switch scenario {
				case "oversize":
					fmt.Fprint(w, strings.Repeat("x", maxSensitive+1))
				case "redirect":
					http.Redirect(w, r, "http://example.invalid/", 302)
				default:
					fmt.Fprint(w, `"sampel-sampel-sampel-sampel\n"`)
				}
			}))
			defer server.Close()
			_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
			dir := t.TempDir()
			ports := port + " insecure loopback\n8080 insecure public\n"
			if scenario == "duplicate listener" {
				ports += "443 secure public\n" + ports
			}
			os.WriteFile(filepath.Join(dir, ".http.ports"), []byte(ports), 0600)
			code, err := readHTTPAccessCode(context.Background(), model.Ship{Name: "~zod", PierPath: dir})
			if scenario == "success" || scenario == "duplicate listener" {
				if err != nil || code != "sampel-sampel-sampel-sampel" {
					t.Fatal("private HTTP failed", err)
				}
			} else if err == nil {
				t.Fatal("accepted", scenario)
			}
		})
	}
}

func TestLoopbackControlURLRejectsConflictingOrInvalidListeners(t *testing.T) {
	for _, ports := range []string{
		"12321 insecure loopback\n12322 insecure loopback\n",
		"12321 insecure loopback\n12321 secure loopback\n",
		"12321 insecure loopback\n0 insecure loopback\n",
		"12321 insecure loopback\n65536 insecure loopback\n",
		"12321 insecure loopback\ninvalid insecure loopback\n",
	} {
		t.Run(strings.TrimSpace(ports), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".http.ports"), []byte(ports), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loopbackControlURL(dir); err == nil {
				t.Fatal("accepted conflicting or invalid private listeners")
			}
		})
	}
}
