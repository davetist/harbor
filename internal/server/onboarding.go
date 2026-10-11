package server

import (
	"harbor/internal/model"
	"harbor/internal/onboarding"
	vr "harbor/internal/runtime"
	"harbor/internal/supervisor"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) AttachRuntime(sup *supervisor.Supervisor, boot *onboarding.Service) {
	s.supervisor = sup
	s.onboarding = boot
}
func (s *Server) runtimeRoutes() {
	s.mux.HandleFunc("GET /api/v1/folders", s.folders)
	s.mux.HandleFunc("POST /api/v1/uploads/keyfile", s.uploadKey)
	s.mux.HandleFunc("POST /api/v1/uploads/archive", s.uploadArchive)
	s.mux.HandleFunc("DELETE /api/v1/uploads/{id}", func(w http.ResponseWriter, r *http.Request) {
		if s.onboarding == nil {
			fail(w, 503, "starting", "Uploads are not ready.")
			return
		}
		if e := s.onboarding.Uploads.Remove(model.ID(r.PathValue("id"))); e != nil {
			fail(w, 404, "not_found", "Upload not found.")
			return
		}
		w.WriteHeader(204)
	})
	s.mux.HandleFunc("POST /api/v1/onboarding/{mode}", s.boot)
	s.mux.HandleFunc("POST /api/v1/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		if s.onboarding == nil {
			fail(w, 503, "starting", "Onboarding is not ready.")
			return
		}
		var body struct {
			UploadID model.ID `json:"uploadId"`
		}
		if err := decode(w, r, &body); err != nil {
			fail(w, 422, "input", "Invalid retry request.")
			return
		}
		id, err := s.onboarding.Retry(r.Context(), model.ID(r.PathValue("id")), body.UploadID)
		if err != nil {
			fail(w, 409, "retry", err.Error())
			return
		}
		respond(w, 202, map[string]model.ID{"jobId": id})
	})
	s.mux.HandleFunc("GET /api/v1/ships/{id}/logs", func(w http.ResponseWriter, r *http.Request) {
		v, ok := s.ship(w, r)
		if !ok {
			return
		}
		if s.supervisor == nil {
			fail(w, 503, "starting", "Terminal is not ready.")
			return
		}
		respond(w, 200, map[string]string{"output": s.supervisor.Logs(v.ID)})
	})
	s.mux.HandleFunc("POST /api/v1/ships/{id}/terminal/input", s.terminalInput)
	s.mux.HandleFunc("POST /api/v1/ships/{id}/terminal/resize", s.resize)
	s.mux.HandleFunc("POST /api/v1/ships/{id}/access-code", s.accessCode)
}
func (s *Server) folders(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		var err error
		path, err = os.UserHomeDir()
		if err != nil {
			fail(w, 422, "path", "The host user's home folder is unavailable.")
			return
		}
	}
	if !filepath.IsAbs(path) {
		fail(w, 422, "path", "Choose an absolute folder path on the Harbor host.")
		return
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		failFolder(w, err)
		return
	}
	entries, err := os.ReadDir(canonical)
	if err != nil {
		failFolder(w, err)
		return
	}
	type directory struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	out := []directory{}
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(canonical, entry.Name())
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		info, err := os.Stat(real)
		if err == nil && info.IsDir() {
			out = append(out, directory{Name: entry.Name(), Path: real})
		}
	}
	var parent *string
	if path := filepath.Dir(canonical); path != canonical {
		parent = &path
	}
	// Browsing reads directory names with the service user's OS permissions.
	// Import, boot and runtime operations retain their separate pier path policy.
	respond(w, 200, struct {
		Path    string      `json:"path"`
		Parent  *string     `json:"parent"`
		Entries []directory `json:"entries"`
	}{canonical, parent, out})
}

func failFolder(w http.ResponseWriter, err error) {
	if os.IsPermission(err) {
		fail(w, 403, "path", "Harbor's OS user cannot open this folder.")
	} else {
		fail(w, 422, "path", "Folder is unavailable.")
	}
}
func (s *Server) uploadKey(w http.ResponseWriter, r *http.Request) {
	if s.onboarding == nil {
		fail(w, 503, "starting", "Uploads are not ready.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	parts, e := r.MultipartReader()
	if e != nil {
		fail(w, 422, "file", "Choose a keyfile.")
		return
	}
	part, e := parts.NextPart()
	if e != nil || part.FormName() != "file" {
		fail(w, 422, "file", "Expected one keyfile.")
		return
	}
	defer part.Close()
	id, e := s.onboarding.Uploads.StageKey(part)
	if e != nil {
		fail(w, 422, "file", e.Error())
		return
	}
	respond(w, 200, map[string]model.ID{"uploadId": id})
}
func (s *Server) boot(w http.ResponseWriter, r *http.Request) {
	if s.onboarding == nil {
		fail(w, 503, "starting", "Onboarding is not ready.")
		return
	}
	var req onboarding.Request
	if e := decode(w, r, &req); e != nil {
		fail(w, 422, "validation", "Invalid onboarding request.")
		return
	}
	var id model.ID
	var e error
	switch r.PathValue("mode") {
	case "comet":
		id, e = s.onboarding.CreateComet(r.Context(), req)
	case "keyfile":
		id, e = s.onboarding.BootKeyfile(r.Context(), req)
	case "existing":
		id, e = s.onboarding.RegisterPier(r.Context(), req)
	default:
		fail(w, 404, "not_found", "Unknown onboarding mode.")
		return
	}
	if e != nil {
		fail(w, 422, "validation", e.Error())
		return
	}
	respond(w, 202, map[string]model.ID{"jobId": id})
}
func (s *Server) terminalInput(w http.ResponseWriter, r *http.Request) {
	v, ok := s.ship(w, r)
	if !ok {
		return
	}
	if s.supervisor == nil {
		fail(w, 503, "starting", "Terminal is not ready.")
		return
	}
	var body struct {
		Input string `json:"input"`
	}
	if e := decode(w, r, &body); e != nil || len(body.Input) > 8192 {
		fail(w, 422, "input", "Terminal input must be no larger than 8 KiB.")
		return
	}
	if e := s.supervisor.WriteInput(r.Context(), v.ID, []byte(body.Input)); e != nil {
		fail(w, 409, "terminal", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) resize(w http.ResponseWriter, r *http.Request) {
	v, ok := s.ship(w, r)
	if !ok {
		return
	}
	if s.supervisor == nil {
		fail(w, 503, "starting", "Terminal is not ready.")
		return
	}
	var body struct {
		Rows uint16 `json:"rows"`
		Cols uint16 `json:"cols"`
	}
	if e := decode(w, r, &body); e != nil {
		fail(w, 422, "input", "Invalid terminal dimensions.")
		return
	}
	if e := s.supervisor.ResizeTerminal(r.Context(), v.ID, body.Rows, body.Cols); e != nil {
		fail(w, 409, "terminal", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) accessCode(w http.ResponseWriter, r *http.Request) {
	v, ok := s.ship(w, r)
	if !ok {
		return
	}
	var body struct{}
	if e := decode(w, r, &body); e != nil {
		fail(w, 422, "input", "Expected an empty request.")
		return
	}
	if v.State != "running" || v.RecoveryBlocked {
		fail(w, 409, "not_ready", "Ship is not ready for a private query.")
		return
	}
	code, e := vr.ReadAccessCode(r.Context(), v)
	if e != nil {
		fail(w, 503, "private_query", "Unable to retrieve the access code. Try again while the ship is running.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]string{"code": code})
}

func (s *Server) uploadArchive(w http.ResponseWriter, r *http.Request) {
	if s.onboarding == nil {
		fail(w, 503, "starting", "Uploads are not ready.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxArchiveBytes+(1<<20))
	parts, e := r.MultipartReader()
	if e != nil {
		fail(w, 422, "archive", "Choose a full Harbor archive.")
		return
	}
	part, e := parts.NextPart()
	if e != nil || part.FormName() != "file" {
		fail(w, 422, "archive", "Expected one full archive.")
		return
	}
	defer part.Close()
	id, e := s.onboarding.Uploads.StageArchive(part, s.cfg.MaxArchiveBytes)
	if e != nil {
		fail(w, 422, "archive", e.Error())
		return
	}
	respond(w, 200, map[string]model.ID{"uploadId": id})
}
