package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"harbor/internal/config"
	"harbor/internal/model"
	"harbor/internal/onboarding"
	vr "harbor/internal/runtime"
	"harbor/internal/scheduler"
	"harbor/internal/store"
	"harbor/internal/supervisor"
	"harbor/internal/ui"
	"harbor/internal/updates"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type Actions interface {
	Submit(context.Context, model.JobRequest) (model.ID, error)
}
type Server struct {
	cfg         config.Config
	store       *store.Store
	actions     Actions
	Events      *Broker
	mux         *http.ServeMux
	supervisor  *supervisor.Supervisor
	onboarding  *onboarding.Service
	version     string
	updateCheck func(context.Context, bool) updates.Status
}

func New(c config.Config, st *store.Store, a Actions) *Server {
	s := &Server{cfg: c, store: st, actions: a, Events: NewBroker(), mux: http.NewServeMux()}
	s.SetVersion("0.1.0-dev")
	s.routes()
	return s
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.allowed(r) {
		fail(w, 403, "origin", "This request is not from the configured Harbor host.")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", contentPolicy(""))
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	s.mux.ServeHTTP(w, r)
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var extra any
	if e := d.Decode(&extra); errors.Is(e, io.EOF) {
		return nil
	}
	return errors.New("one JSON object required")
}
func (s *Server) routes() {
	s.updateRoutes()
	s.runtimeRoutes()
	s.scheduleRoutes()
	s.backupRoutes()
	s.recoveryRoutes()
	s.mux.HandleFunc("GET /api/v1/host", func(w http.ResponseWriter, r *http.Request) {
		name, _ := os.Hostname()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		resources, resourceErr := scheduler.ReadResources(ctx, s.cfg.AllowedRoots[0])
		resources.LoomExponent = 31
		resources.RuntimeVersion = vr.StableVersion
		target, _ := vr.NativePlatform()
		_, profileErr := scheduler.ProfileFor(target, resources)
		validated := resourceErr == nil && profileErr == nil
		description := "Starts paused · no measured automatic maintenance profile for this host"
		if validated {
			description = "Enabled by default · weekly Pack and Roll/Chop · 28-day Meld when the snapshot reaches 512 MiB · retains two epochs · resource checks before every run"
		}
		respond(w, 200, map[string]any{"hostname": name, "harborVersion": s.version, "managerPID": os.Getpid(), "platform": runtime.GOOS + "/" + runtime.GOARCH, "dataDir": s.cfg.DataDir, "pierRoot": s.cfg.AllowedRoots[0], "allowedRoots": s.cfg.AllowedRoots, "timezone": s.cfg.Timezone, "loomSizes": []int{30, 31, 32}, "defaultShipHTTPBindAddress": s.cfg.DefaultShipHTTPBindAddress, "maintenanceProfile": description, "maintenanceValidated": validated})
	})
	s.mux.HandleFunc("GET /api/v1/ships", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.store.ListShips(r.Context())
		if e != nil {
			fail(w, 500, "storage", "Unable to read ships.")
			return
		}
		respond(w, 200, v)
	})
	s.mux.HandleFunc("GET /api/v1/ships/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, ok := s.ship(w, r)
		if ok {
			respond(w, 200, v)
		}
	})
	s.mux.HandleFunc("DELETE /api/v1/ships/{id}", s.removeShip)
	s.mux.HandleFunc("POST /api/v1/ships/{id}/measure", s.measurePier)
	s.mux.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.store.GetJob(r.Context(), model.ID(r.PathValue("id")))
		if errors.Is(e, store.ErrNotFound) {
			fail(w, 404, "not_found", "Job not found.")
			return
		}
		if e != nil {
			fail(w, 500, "storage", "Unable to read job.")
			return
		}
		v.Retryable = s.store.CanRetryPrelaunch(r.Context(), v)
		respond(w, 200, v)
	})
	s.mux.HandleFunc("GET /api/v1/ships/{id}/jobs", func(w http.ResponseWriter, r *http.Request) {
		ship, ok := s.ship(w, r)
		if !ok {
			return
		}
		rows, err := s.store.JobsForShip(r.Context(), ship.ID)
		if err != nil {
			fail(w, 500, "storage", "Unable to read job history.")
			return
		}
		respond(w, 200, rows)
	})
	s.mux.HandleFunc("GET /api/v1/jobs/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		id := model.ID(r.PathValue("id"))
		if _, err := s.store.GetJob(r.Context(), id); err != nil {
			fail(w, 404, "not_found", "Job not found.")
			return
		}
		if s.supervisor != nil && !s.supervisor.JobLogAvailable(id) {
			fail(w, 503, "diagnostics", "Some output could not be retained; inspect the live terminal.")
			return
		}
		output, err := s.store.JobLog(r.Context(), id)
		if err != nil {
			fail(w, 500, "storage", "Unable to read retained output.")
			return
		}
		respond(w, 200, map[string]string{"output": string(output)})
	})
	s.mux.HandleFunc("GET /api/v1/ships/{id}/open", s.open)
	s.mux.HandleFunc("POST /api/v1/ships/{id}/{operation}", s.action)
	s.mux.HandleFunc("GET /api/v1/events", s.events)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not_found", "Endpoint not found.") })
	s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			fail(w, 405, "method", "Method not allowed.")
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			s.index(w, r)
			return
		}
		http.FileServerFS(ui.Files()).ServeHTTP(w, r)
	}))
}
func (s *Server) ship(w http.ResponseWriter, r *http.Request) (model.Ship, bool) {
	v, e := s.store.GetShip(r.Context(), model.ID(r.PathValue("id")))
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "not_found", "Ship not found.")
		return v, false
	}
	if e != nil {
		fail(w, 500, "storage", "Unable to read ship.")
		return v, false
	}
	return v, true
}
func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	v, ok := s.ship(w, r)
	if !ok {
		return
	}
	var body struct{}
	if e := decode(w, r, &body); e != nil {
		fail(w, 422, "validation", "Expected an empty action object.")
		return
	}
	op := model.Operation(r.PathValue("operation"))
	if v.RecoveryBlocked && op != "backup" {
		fail(w, 409, "recovery_hold", v.RecoveryBlockReason)
		return
	}
	if s.actions == nil {
		fail(w, 503, "starting", "Harbor's process controller is not ready.")
		return
	}

	switch op {
	case "start", "stop", "pack", "meld", "roll", "chop", "update", "replay", "backup":
	default:
		fail(w, 404, "not_found", "Operation not found.")
		return
	}
	id, e := s.actions.Submit(r.Context(), model.JobRequest{ShipID: v.ID, Kind: op, Trigger: "manual"})
	if e != nil {
		fail(w, 409, "conflict", e.Error())
		return
	}
	respond(w, 202, map[string]model.ID{"jobId": id})
}
func (s *Server) open(w http.ResponseWriter, r *http.Request) {
	v, ok := s.ship(w, r)
	if !ok {
		return
	}
	if v.State != "running" {
		fail(w, 409, "not_ready", "The ship is not ready.")
		return
	}
	host := s.cfg.AdvertisedHost
	if host == "" {
		host, _, _ = net.SplitHostPort(r.Host)
	}
	respond(w, 200, map[string]string{"url": fmt.Sprintf("http://%s", net.JoinHostPort(host, fmt.Sprint(v.WebPort)))})
}
