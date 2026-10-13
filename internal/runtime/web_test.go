package runtime

import (
	"context"
	"harbor/internal/model"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestCheckWebUsesSavedListenerWithoutProxyOrRedirect(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("response check followed a redirect")
	}))
	defer target.Close()
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/~/login" || r.Header.Get("Cookie") != "" {
			t.Error("response check was not an unauthenticated public login request")
		}
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer web.Close()
	_, port, _ := net.SplitHostPort(web.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	for _, bind := range []string{"127.0.0.1", "0.0.0.0"} {
		if err := CheckWeb(context.Background(), model.Ship{HTTPBindAddress: bind, WebPort: p}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckWebRejectsFailureAndBoundsWholeResponse(t *testing.T) {
	for _, mode := range []string{"status", "headers", "body", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "status":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "headers":
					<-r.Context().Done()
				case "body":
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				case "truncated":
					w.Header().Set("Content-Length", "100")
					w.Write([]byte("short"))
				}
			}))
			defer web.Close()
			_, port, _ := net.SplitHostPort(web.Listener.Addr().String())
			p, _ := strconv.Atoi(port)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			if err := CheckWeb(ctx, model.Ship{HTTPBindAddress: "127.0.0.1", WebPort: p}); err == nil {
				t.Fatal("unresponsive or incomplete reply accepted")
			}
			if time.Since(start) > time.Second {
				t.Fatal("response check ignored its deadline")
			}
		})
	}
}

func TestCheckWebRejectsInvalidSavedAddress(t *testing.T) {
	for _, ship := range []model.Ship{
		{HTTPBindAddress: "evil.invalid", WebPort: 8080},
		{HTTPBindAddress: "127.0.0.1", WebPort: 0},
		{HTTPBindAddress: "127.0.0.1", WebPort: 65536},
	} {
		if err := CheckWeb(context.Background(), ship); err == nil {
			t.Fatal("invalid listener accepted")
		}
	}
}
