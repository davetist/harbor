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
	for _, scenario := range []string{"success", "wrong identity", "oversize", "redirect"} {
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
			os.WriteFile(filepath.Join(dir, ".http.ports"), []byte(port+" insecure loopback\n8080 insecure public\n"), 0600)
			code, err := readHTTPAccessCode(context.Background(), model.Ship{Name: "~zod", PierPath: dir})
			if scenario == "success" {
				if err != nil || code != "sampel-sampel-sampel-sampel" {
					t.Fatal("private HTTP failed", err)
				}
			} else if err == nil {
				t.Fatal("accepted", scenario)
			}
		})
	}
}
