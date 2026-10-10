package server

import (
	"harbor/internal/model"
	"harbor/internal/scheduler"
	"net/http"
	"time"
)

func (s *Server) scheduleRoutes() {
	s.mux.HandleFunc("GET /api/v1/ships/{id}/schedules", func(w http.ResponseWriter, r *http.Request) {
		ship, ok := s.ship(w, r)
		if !ok {
			return
		}
		rows, e := s.store.ListSchedules(r.Context(), ship.ID)
		if e != nil {
			fail(w, 500, "storage", "Unable to read schedules.")
			return
		}
		jobs, e := s.store.JobsForShip(r.Context(), ship.ID)
		if e != nil {
			fail(w, 500, "storage", "Unable to read operation results.")
			return
		}
		for i, row := range rows {
			rows[i].LastResult = "Never run"
			for _, job := range jobs {
				if job.Kind == row.Kind {
					rows[i].LastResult = job.Phase
					if job.Error != "" {
						rows[i].LastResult += " · " + job.Error
					}
					break
				}
			}
		}
		respond(w, 200, rows)
	})
	s.mux.HandleFunc("PUT /api/v1/ships/{id}/schedules", func(w http.ResponseWriter, r *http.Request) {
		ship, ok := s.ship(w, r)
		if !ok {
			return
		}
		var row model.Schedule
		if e := decode(w, r, &row); e != nil || row.ShipID != ship.ID {
			fail(w, 422, "schedule", "Invalid schedule request.")
			return
		}
		existing, e := s.store.ListSchedules(r.Context(), ship.ID)
		if e != nil {
			fail(w, 500, "storage", "Unable to read schedules.")
			return
		}
		found := false
		for _, v := range existing {
			if v.ID == row.ID {
				found = true
				if row.AnchorDate == "" {
					row.AnchorDate = v.AnchorDate
				}
				if row.AfterScheduleID == "" {
					row.AfterScheduleID = v.AfterScheduleID
				}
			}
		}
		if !found {
			fail(w, 404, "not_found", "Schedule not found.")
			return
		}
		if e = scheduler.Validate(row); e != nil {
			fail(w, 422, "schedule", e.Error())
			return
		}
		row.NextDue, e = scheduler.NextOccurrence(row, time.Now())
		if e != nil {
			fail(w, 422, "schedule", e.Error())
			return
		}
		if e = s.store.EditSchedule(r.Context(), row, func(v model.Schedule) (time.Time, error) { return scheduler.NextOccurrence(v, time.Now()) }); e != nil {
			fail(w, 422, "schedule", e.Error())
			return
		}
		s.Events.Publish(model.Event{Type: "schedule", ShipID: ship.ID, Data: row})
		w.WriteHeader(204)
	})
}
