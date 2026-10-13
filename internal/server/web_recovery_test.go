package server

import (
	"context"
	"errors"
	"harbor/internal/jobs"
	"harbor/internal/model"
	"harbor/internal/supervisor"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenUnresponsiveShipDoesNotOfferBrokenURL(t *testing.T) {
	h := handler(t)
	ship := model.Ship{ID: "busy", PierPath: "/test/pier", HTTPBindAddress: "127.0.0.1", WebPort: 18080, State: "running", DesiredRunning: true}
	if err := h.store.PutShip(context.Background(), ship); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8789/api/v1/ships/busy/open", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"ship_unresponsive"`) || strings.Contains(w.Body.String(), `"url"`) {
		t.Fatalf("unresponsive ship was opened: %d %s", w.Code, w.Body.String())
	}
	current, err := h.store.GetShip(context.Background(), ship.ID)
	if err != nil || current.State != "running" || !current.DesiredRunning || current.RecoveryBlocked {
		t.Fatal("response check changed lifecycle state", current.State, err)
	}
}

type interruptActions struct {
	calls int
	err   error
}

func (a *interruptActions) Submit(context.Context, model.JobRequest) (model.ID, error) {
	return "", errors.New("not a lifecycle job")
}
func (a *interruptActions) InterruptShip(_ context.Context, id model.ID) error {
	if id != "busy" {
		return errors.New("wrong ship")
	}
	a.calls++
	return a.err
}

func TestInterruptRouteRequiresExplicitValidSameOriginRequest(t *testing.T) {
	for _, tc := range []struct {
		name, body, origin, id string
		controller             bool
		err                    error
		status, calls          int
	}{
		{"send", `{}`, "", "busy", true, nil, 204, 1},
		{"unknown field", `{"force":true}`, "", "busy", true, nil, 422, 0},
		{"no confirmation object", ``, "", "busy", true, nil, 422, 0},
		{"cross origin", `{}`, "http://evil.invalid", "busy", true, nil, 403, 0},
		{"missing ship", `{}`, "", "missing", true, nil, 404, 0},
		{"no controller", `{}`, "", "busy", false, nil, 503, 0},
		{"responding again", `{}`, "", "busy", true, supervisor.ErrWebResponsive, 409, 1},
		{"conflict", `{}`, "", "busy", true, errors.New("queued operation"), 409, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := handler(t)
			if err := h.store.PutShip(context.Background(), model.Ship{ID: "busy", PierPath: "/test/pier", State: "running"}); err != nil {
				t.Fatal(err)
			}
			a := &interruptActions{err: tc.err}
			if tc.controller {
				h.actions = a
			}
			r := httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/ships/"+tc.id+"/interrupt", strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || a.calls != tc.calls {
				t.Fatalf("%d calls=%d body=%s", w.Code, a.calls, w.Body.String())
			}
			if tc.err == supervisor.ErrWebResponsive && !strings.Contains(w.Body.String(), `"ship_responsive"`) {
				t.Fatal("lost responsive-again result")
			}
		})
	}
}

func TestInterruptControllerRefusesDuringRecoveryOrShutdown(t *testing.T) {
	h := handler(t)
	if err := h.store.PutShip(context.Background(), model.Ship{ID: "busy", PierPath: "/test/pier", State: "running"}); err != nil {
		t.Fatal(err)
	}
	h.actions = jobs.New(h.store, supervisor.New(h.store, nil))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/ships/busy/interrupt", strings.NewReader(`{}`)))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "recovering or shutting down") {
		t.Fatalf("unready controller accepted interruption: %d %s", w.Code, w.Body.String())
	}
}
