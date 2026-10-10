package server

import (
	"fmt"
	"harbor/internal/model"
	"net/http"
)

func (s *Server) backupRoutes() {
	s.mux.HandleFunc("GET /api/v1/ships/{id}/backups", func(w http.ResponseWriter, r *http.Request) {
		ship, ok := s.ship(w, r)
		if !ok {
			return
		}
		rows, e := s.store.ListBackups(r.Context(), ship.ID)
		if e != nil {
			fail(w, 500, "storage", "Unable to read pier exports.")
			return
		}
		respond(w, 200, rows)
	})
	s.mux.HandleFunc("GET /api/v1/backups/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		b, e := s.store.GetBackup(r.Context(), model.ID(r.PathValue("id")))
		if e != nil {
			fail(w, 404, "not_found", "Pier export not found.")
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="harbor-export-%s.tar.gz"`, b.ID))
		http.ServeFile(w, r, b.ArchivePath)
	})
}
