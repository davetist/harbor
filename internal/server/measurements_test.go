package server

import (
	"context"
	"encoding/json"
	"harbor/internal/model"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMeasurePierRefreshesStoredSizeWithoutChangingShipState(t *testing.T) {
	h := handler(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := h.store.PutShip(context.Background(), model.Ship{ID: "measured", PierPath: root, State: "running", DesiredRunning: true}); err != nil {
		t.Fatal(err)
	}
	events, cancel := h.Events.Subscribe()
	defer cancel()
	for _, size := range []int64{5, 11} {
		if size == 11 {
			if err := os.WriteFile(filepath.Join(root, "more"), []byte(" world"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		before := time.Now()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/ships/measured/measure", strings.NewReader(`{}`)))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var ship model.Ship
		if err := json.Unmarshal(w.Body.Bytes(), &ship); err != nil {
			t.Fatal(err)
		}
		if ship.PierBytes == nil || *ship.PierBytes != size || ship.MeasuredAt.Before(before) || ship.State != "running" || !ship.DesiredRunning {
			t.Fatalf("unexpected measurement: %+v", ship)
		}
		stored, err := h.store.GetShip(context.Background(), "measured")
		if err != nil || stored.PierBytes == nil || *stored.PierBytes != size || !stored.MeasuredAt.Equal(ship.MeasuredAt) {
			t.Fatalf("measurement was not persisted: %+v %v", stored, err)
		}
		select {
		case event := <-events:
			if event.Type != "ship" || event.ShipID != "measured" {
				t.Fatalf("unexpected event: %+v", event)
			}
		default:
			t.Fatal("measurement did not notify connected clients")
		}
	}
}

func TestMeasurePierFailurePreservesPreviousMeasurement(t *testing.T) {
	h := handler(t)
	root := t.TempDir()
	size, at := int64(42), time.Now().Add(-time.Hour).UTC()
	if err := h.store.PutShip(context.Background(), model.Ship{ID: "measured", PierPath: root, PierBytes: &size, MeasuredAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/ships/measured/measure", strings.NewReader(`{}`)))
	if w.Code != 409 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	ship, err := h.store.GetShip(context.Background(), "measured")
	if err != nil || ship.PierBytes == nil || *ship.PierBytes != 42 || !ship.MeasuredAt.Equal(at) {
		t.Fatalf("failed measurement replaced the previous value: %+v %v", ship, err)
	}
}

func TestMeasurePierRejectsUnknownShipAndInvalidRequest(t *testing.T) {
	h := handler(t)
	if err := h.store.PutShip(context.Background(), model.Ship{ID: "measured", PierPath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	for _, trial := range []struct {
		id, body string
		status   int
	}{{"missing", `{}`, 404}, {"measured", `{"path":"/"}`, 422}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/ships/"+trial.id+"/measure", strings.NewReader(trial.body)))
		if w.Code != trial.status {
			t.Fatalf("%s: %d %s", trial.id, w.Code, w.Body.String())
		}
	}
}
