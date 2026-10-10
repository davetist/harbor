package server

import (
	"context"
	"encoding/json"
	"harbor/internal/model"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPairEditAndManualResultUseAuthoritativeState(t *testing.T) {
	h := handler(t)
	ctx := context.Background()
	h.store.PutShip(ctx, model.Ship{ID: "s", PierPath: "/test/pier"})
	roll := model.Schedule{ID: "r", ShipID: "s", Kind: "roll", AnchorDate: "2026-10-01", IntervalDays: 7, LocalTime: "02:00", Timezone: "UTC"}
	chop := roll
	chop.ID = "c"
	chop.Kind = "chop"
	chop.LocalTime = "03:00"
	chop.AfterScheduleID = "r"
	h.store.PutSchedule(ctx, roll)
	h.store.PutSchedule(ctx, chop)
	roll.IntervalDays = 14
	b, _ := json.Marshal(roll)
	r := httptest.NewRequest("PUT", "http://127.0.0.1:8789/api/v1/ships/s/schedules", strings.NewReader(string(b)))
	r.Header.Set("Origin", "http://127.0.0.1:8789")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	j := model.Job{ID: "manual", ShipID: "s", Kind: "roll", Trigger: "manual", Phase: "queued"}
	h.store.CreateJob(ctx, j)
	j.Phase = "failed"
	j.Error = "native failure"
	h.store.UpdateJob(ctx, j)
	r = httptest.NewRequest("GET", "http://127.0.0.1:8789/api/v1/ships/s/schedules", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var rows []model.Schedule
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.IntervalDays != 14 {
			t.Fatal("pair cadence stale")
		}
		if row.Kind == "roll" && !strings.Contains(row.LastResult, "native failure") {
			t.Fatal("manual failure hidden")
		}
		if row.LastJobID != "" {
			t.Fatal("manual result overwrote occurrence identity")
		}
	}
}

func TestUnrunScheduleDoesNotShowProvisionalProfileAsJobResult(t *testing.T) {
	h := handler(t)
	ctx := context.Background()
	if err := h.store.PutShip(ctx, model.Ship{ID: "s", PierPath: "/test/pier"}); err != nil {
		t.Fatal(err)
	}
	row := model.Schedule{ID: "m", ShipID: "s", Kind: "meld", AnchorDate: "2026-10-08", IntervalDays: 28, LocalTime: "03:15", Timezone: "UTC", Enabled: true, LastResult: "Awaiting measured default profile"}
	if err := h.store.PutSchedule(ctx, row); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8789/api/v1/ships/s/schedules", nil))
	var rows []model.Schedule
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LastResult != "Never run" || !rows[0].Enabled {
		t.Fatal("unrun schedule showed a profile label as a job result", rows)
	}
}
