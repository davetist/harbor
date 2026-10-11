package server

import (
	"encoding/json"
	"fmt"
	"harbor/internal/onboarding"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type browserListing struct {
	Path    string  `json:"path"`
	Parent  *string `json:"parent"`
	Entries []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"entries"`
}

func readFolders(t *testing.T, h *Server, path string) browserListing {
	t.Helper()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8789/api/v1/folders?"+url.Values{"path": {path}}.Encode(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("cannot browse %q: %d %s", path, w.Code, w.Body.String())
	}
	var result browserListing
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("folder browser listing: %v", err)
	}
	return result
}

func TestFolderBrowserNavigatesTheActualHostTree(t *testing.T) {
	h := handler(t)
	host, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	piers := filepath.Join(host, "piers")
	sibling := filepath.Join(host, "another folder & spaces")
	for _, path := range []string{piers, filepath.Join(sibling, "child"), filepath.Join(host, ".hidden")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(host, "ordinary-file"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sibling, filepath.Join(piers, "linked-folder")); err != nil {
		t.Fatal(err)
	}
	h.cfg.AllowedRoots = []string{piers}
	t.Setenv("HOME", host)
	first := readFolders(t, h, piers)
	if first.Path != piers || first.Parent == nil || *first.Parent != host || len(first.Entries) != 1 || first.Entries[0].Path != sibling {
		t.Fatalf("cannot navigate to the parent or linked folder: %+v", first)
	}
	listing := readFolders(t, h, *first.Parent)
	names := map[string]bool{}
	for _, entry := range listing.Entries {
		names[entry.Name] = true
	}
	if !names["another folder & spaces"] || !names[".hidden"] || names["ordinary-file"] {
		t.Fatalf("listing did not reflect actual directories: %+v", names)
	}
	child := readFolders(t, h, sibling)
	if len(child.Entries) != 1 || child.Entries[0].Name != "child" || child.Parent == nil || *child.Parent != host {
		t.Fatalf("child navigation: %+v", child)
	}
	if empty := readFolders(t, h, filepath.Join(sibling, "child")); empty.Entries == nil || len(empty.Entries) != 0 {
		t.Fatalf("empty directory: %+v", empty)
	}
	if home := readFolders(t, h, ""); home.Path != host {
		t.Fatalf("Home opened %q instead of the host user's home", home.Path)
	}
	if root := readFolders(t, h, "/"); root.Path != "/" || root.Parent != nil {
		t.Fatalf("filesystem root parent: %+v", root)
	}
	newFolder := filepath.Join(sibling, "created after browsing")
	if err := os.Mkdir(newFolder, 0700); err != nil {
		t.Fatal(err)
	}
	if refreshed := readFolders(t, h, sibling); len(refreshed.Entries) != 2 {
		t.Fatalf("listing is stale or fixed: %+v", refreshed)
	}
}

func TestFolderBrowsingDoesNotAdmitAnImportOutsideStorageRoots(t *testing.T) {
	h := handler(t)
	root := t.TempDir()
	piers, outside := filepath.Join(root, "piers"), filepath.Join(root, "outside")
	os.Mkdir(piers, 0700)
	os.Mkdir(outside, 0700)
	h.cfg.AllowedRoots = []string{piers}
	readFolders(t, h, outside)
	h.AttachRuntime(nil, &onboarding.Service{Config: h.cfg})
	body, _ := json.Marshal(map[string]any{"pierPath": outside, "loomExponent": 31, "loomConfirmed": true})
	r := httptest.NewRequest("POST", "http://127.0.0.1:8789/api/v1/onboarding/existing", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "outside Harbor") {
		t.Fatalf("browsing authorized an import: %d %s", w.Code, w.Body.String())
	}
}

func TestFolderBrowserDoesNotSilentlyHideFoldersAfter256Entries(t *testing.T) {
	h := handler(t)
	root := t.TempDir()
	for i := range 300 {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("folder-%03d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if result := readFolders(t, h, root); len(result.Entries) != 300 {
		t.Fatalf("folders omitted: %d", len(result.Entries))
	}
}

func TestFolderBrowserReportsUnavailablePaths(t *testing.T) {
	h := handler(t)
	root := t.TempDir()
	file := filepath.Join(root, "file")
	os.WriteFile(file, []byte("fixture"), 0600)
	for _, path := range []string{"relative/path", filepath.Join(root, "missing"), file} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8789/api/v1/folders?"+url.Values{"path": {path}}.Encode(), nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 422 || !strings.Contains(w.Body.String(), `"error"`) {
			t.Fatalf("invalid folder %q: %d %s", path, w.Code, w.Body.String())
		}
	}
}
