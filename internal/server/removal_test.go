package server

import (
	"context"
	"encoding/json"
	"errors"
	"harbor/internal/jobs"
	"harbor/internal/model"
	"harbor/internal/pier"
	"harbor/internal/store"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRemoveShipCanPermanentlyDeleteConfirmedPier(t *testing.T) {
	h, ship := removalHandler(t)
	archive := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(archive, []byte("keep export"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.store.PutBackup(context.Background(), model.Backup{ID: "export", ShipID: ship.ID, ArchivePath: archive, CreatedAt: time.Now(), Verified: true}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"deletePier": true, "confirmPierPath": ship.PierPath})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "http://127.0.0.1:8789/api/v1/ships/test-comet", strings.NewReader(string(body))))
	if w.Code != 204 {
		t.Fatalf("remove: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Lstat(ship.PierPath); !os.IsNotExist(err) {
		t.Fatalf("confirmed pier was not deleted: %v", err)
	}
	if b, err := os.ReadFile(archive); err != nil || string(b) != "keep export" {
		t.Fatalf("export changed: %v", err)
	}
}

func TestPermanentRemovalRequiresExactPathConfirmation(t *testing.T) {
	for _, body := range []string{`{"deletePier":true}`, `{"deletePier":true,"confirmPierPath":"/wrong/pier"}`, `{"deletePier":"yes"}`, `{"deletePier":true,"unknown":true}`, `{} {}`} {
		t.Run(body, func(t *testing.T) {
			h, ship := removalHandler(t)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("DELETE", "http://127.0.0.1:8789/api/v1/ships/test-comet", strings.NewReader(body)))
			if w.Code != 400 {
				t.Fatalf("invalid confirmation accepted: %d %s", w.Code, w.Body.String())
			}
			if _, err := h.store.GetShip(context.Background(), ship.ID); err != nil {
				t.Fatalf("registration changed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(ship.PierPath, "pier-data")); err != nil {
				t.Fatalf("pier changed: %v", err)
			}
		})
	}
}

func TestPermanentRemovalPreservesProtectedFoldersAndBusyPiers(t *testing.T) {
	for _, scenario := range []string{"guard", "recovery", "running", "job", "process", "nested-pier", "parent-pier", "export", "harbor-data", "aliased-data", "aliased-pier"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			h, ship := removalHandler(t)
			switch scenario {
			case "guard":
				release, err := pier.Guard(ship.PierPath)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
			case "recovery":
				ship.RecoveryBlocked = true
			case "running":
				ship.State = "running"
				ship.DesiredRunning = true
			case "job":
				if err := h.store.CreateJob(ctx, model.Job{ID: "queued", ShipID: ship.ID, Kind: "start", Trigger: "manual", Phase: "queued"}); err != nil {
					t.Fatal(err)
				}
			case "process":
				if err := h.store.PutProcess(ctx, model.ProcessRecord{LaunchID: "uncertain", ShipID: ship.ID, Phase: "launching"}); err != nil {
					t.Fatal(err)
				}
			case "nested-pier", "parent-pier":
				otherPath := filepath.Join(ship.PierPath, "nested")
				if scenario == "parent-pier" {
					otherPath = filepath.Dir(ship.PierPath)
				}
				if err := h.store.PutShip(ctx, model.Ship{ID: "other", PierPath: otherPath, State: "running"}); err != nil {
					t.Fatal(err)
				}
			case "export":
				if err := h.store.PutBackup(ctx, model.Backup{ID: "inside", ShipID: ship.ID, ArchivePath: filepath.Join(ship.PierPath, "archive.tar.gz"), CreatedAt: time.Now()}); err != nil {
					t.Fatal(err)
				}
			case "harbor-data":
				h.cfg.DataDir = filepath.Join(ship.PierPath, "data")
			case "aliased-data", "aliased-pier":
				alias := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(ship.PierPath, alias); err != nil {
					t.Fatal(err)
				}
				if scenario == "aliased-data" {
					h.cfg.DataDir = alias
				} else {
					if err := os.Mkdir(filepath.Join(ship.PierPath, "nested"), 0700); err != nil {
						t.Fatal(err)
					}
					if err := h.store.PutShip(ctx, model.Ship{ID: "other", PierPath: filepath.Join(alias, "nested"), State: "running"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := h.store.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(map[string]any{"deletePier": true, "confirmPierPath": ship.PierPath})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("DELETE", "http://127.0.0.1:8789/api/v1/ships/test-comet", strings.NewReader(string(body))))
			if w.Code != 409 {
				t.Fatalf("unsafe removal accepted: %d %s", w.Code, w.Body.String())
			}
			if _, err := h.store.GetShip(ctx, ship.ID); err != nil {
				t.Fatalf("registration removed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(ship.PierPath, "pier-data")); err != nil {
				t.Fatalf("pier files removed: %v", err)
			}
		})
	}
}

func removalHandler(t *testing.T) (*Server, model.Ship) {
	t.Helper()
	h := handler(t)
	executor := jobs.New(h.store, nil)
	executor.SetReady(true)
	executor.OnChange = h.Events.Publish
	h.actions = executor
	path := filepath.Join(t.TempDir(), "test-comet")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "pier-data"), []byte("preserve this pier"), 0600); err != nil {
		t.Fatal(err)
	}
	ship := model.Ship{ID: "test-comet", Name: "Test comet", PierPath: path, State: "stopped"}
	if err := h.store.PutShip(context.Background(), ship); err != nil {
		t.Fatal(err)
	}
	ship, err := h.store.GetShip(context.Background(), ship.ID)
	if err != nil {
		t.Fatal(err)
	}
	return h, ship
}

func TestRemoveShipPreservesFilesAndAllowsRegistrationAgain(t *testing.T) {
	ctx := context.Background()
	h, ship := removalHandler(t)
	other := model.Ship{ID: "other", PierPath: filepath.Join(t.TempDir(), "other"), State: "stopped"}
	if err := h.store.PutShip(ctx, other); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	schedule := model.Schedule{ID: "pack", ShipID: ship.ID, Kind: "pack", Enabled: true, NextDue: now, IntervalDays: 7, AnchorDate: "2026-01-01", LocalTime: "03:00", Timezone: "UTC"}
	if err := h.store.PutSchedule(ctx, schedule); err != nil {
		t.Fatal(err)
	}
	queued, err := h.store.ClaimDueAndEnqueue(ctx, now, func(model.Schedule, time.Time) (time.Time, error) { return now.Add(7 * 24 * time.Hour), nil })
	if err != nil || len(queued) != 1 {
		t.Fatalf("scheduled history: %v %v", queued, err)
	}
	job := queued[0]
	job.Phase = "completed"
	if err := h.store.UpdateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := h.store.AppendJobLog(ctx, job.ID, []byte("completed output")); err != nil {
		t.Fatal(err)
	}
	if err := h.store.PutProcess(ctx, model.ProcessRecord{LaunchID: "old-process", ShipID: ship.ID, JobID: job.ID, Phase: "exited"}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "export.tar.gz")
	if err := os.WriteFile(archive, []byte("preserve this export"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.store.PutBackup(ctx, model.Backup{ID: "export", ShipID: ship.ID, ArchivePath: archive, CreatedAt: now, Verified: true}); err != nil {
		t.Fatal(err)
	}
	events, cancel := h.Events.Subscribe()
	defer cancel()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "http://127.0.0.1:8789/api/v1/ships/test-comet", nil))
	if w.Code != 204 {
		t.Fatalf("remove: %d %s", w.Code, w.Body.String())
	}
	if _, err := h.store.GetShip(ctx, ship.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ship remains: %v", err)
	}
	if _, err := h.store.GetShip(ctx, other.ID); err != nil {
		t.Fatalf("other ship changed: %v", err)
	}
	for path, want := range map[string]string{filepath.Join(ship.PierPath, "pier-data"): "preserve this pier", archive: "preserve this export"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("file changed: %q %v", data, err)
		}
	}
	select {
	case event := <-events:
		if event.Type != "ship" || event.ShipID != ship.ID {
			t.Fatalf("unexpected removal event: %+v", event)
		}
	default:
		t.Fatal("removal did not notify other browsers")
	}
	ship.ID = "registered-again"
	if err := h.store.PutShip(ctx, ship); err != nil {
		t.Fatalf("pier cannot be registered again: %v", err)
	}
	rows, err := h.store.ListSchedules(ctx, "test-comet")
	if err != nil || len(rows) != 0 {
		t.Fatalf("maintenance remains: %v %v", rows, err)
	}
}

func TestRemoveShipRefusesUnsafeStates(t *testing.T) {
	for _, scenario := range []string{"running", "starting", "stopping", "maintaining", "externally_running", "autostart", "recovery", "queued", "executing", "process", "unready"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			h, ship := removalHandler(t)
			switch scenario {
			case "autostart":
				ship.DesiredRunning = true
			case "recovery":
				ship.RecoveryBlocked = true
			case "queued", "executing":
				job := model.Job{ID: "busy", ShipID: ship.ID, Kind: "pack", Trigger: "manual", Phase: "queued"}
				if err := h.store.CreateJob(ctx, job); err != nil {
					t.Fatal(err)
				}
				if scenario == "executing" {
					job.Phase = "executing"
					if err := h.store.UpdateJob(ctx, job); err != nil {
						t.Fatal(err)
					}
				}
			case "process":
				if err := h.store.PutProcess(ctx, model.ProcessRecord{LaunchID: "live-process", ShipID: ship.ID, Phase: "running"}); err != nil {
					t.Fatal(err)
				}
			case "unready":
				h.actions.(*jobs.Executor).SetReady(false)
			default:
				ship.State = scenario
			}
			if err := h.store.PutShip(ctx, ship); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("DELETE", "http://127.0.0.1:8789/api/v1/ships/test-comet", nil))
			if w.Code != 409 {
				t.Fatalf("remove %s: %d %s", scenario, w.Code, w.Body.String())
			}
			if _, err := h.store.GetShip(ctx, ship.ID); err != nil {
				t.Fatalf("unsafe removal changed registry: %v", err)
			}
		})
	}
}

func TestRemoveShipMissingControllerAndCrossOrigin(t *testing.T) {
	for _, scenario := range []string{"missing", "controller", "origin"} {
		t.Run(scenario, func(t *testing.T) {
			h, _ := removalHandler(t)
			id, want := "test-comet", 503
			if scenario == "missing" {
				id, want = "missing", 404
			} else if scenario == "controller" {
				h.actions = nil
			} else {
				want = 403
			}
			r := httptest.NewRequest("DELETE", "http://127.0.0.1:8789/api/v1/ships/"+id, nil)
			if scenario == "origin" {
				r.Header.Set("Origin", "https://evil.invalid")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}
