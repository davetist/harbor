package pier

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Canonical(path string, roots []string, newDestination bool) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("choose an absolute path on the Harbor host")
	}
	path = filepath.Clean(path)
	real, e := filepath.EvalSymlinks(path)
	if newDestination {
		if e == nil {
			return "", errors.New("destination already exists")
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			return "", err
		}
		real = filepath.Join(parent, filepath.Base(path))
	} else if e != nil {
		return "", e
	}
	for _, root := range roots {
		r, e := filepath.EvalSymlinks(root)
		if e != nil {
			continue
		}
		relative, e := filepath.Rel(r, real)
		if e == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative) {
			return real, nil
		}
	}
	return "", errors.New("path is outside Harbor's permitted folders")
}

type Inspection struct {
	Path        string
	HasSnapshot bool
	HasEvents   bool
}

// VerifyRegistered rejects path drift; it never adopts a new symlink target.
func VerifyRegistered(p string, roots []string, newDestination bool) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return errors.New("invalid registered pier path")
	}
	var real string
	var err error
	if len(roots) != 0 {
		real, err = Canonical(p, roots, newDestination)
	} else if newDestination {
		if _, err = os.Lstat(p); !os.IsNotExist(err) {
			return errors.New("boot destination already exists")
		}
		var parent string
		parent, err = filepath.EvalSymlinks(filepath.Dir(p))
		real = filepath.Join(parent, filepath.Base(p))
	} else {
		real, err = filepath.EvalSymlinks(p)
	}
	if err != nil || real != p {
		return errors.New("registered pier path changed or is outside permitted folders")
	}
	if newDestination {
		return nil
	}
	for _, name := range []string{".urb", ".urb/chk", ".urb/log", ".urb/put", ".vere.lock", ".http.ports"} {
		target, e := filepath.EvalSymlinks(filepath.Join(p, name))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(p, target)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return errors.New("pier storage path escapes registered folder")
		}
	}
	return nil
}

func Inspect(path string) (Inspection, error) {
	v := Inspection{Path: path}
	st, e := os.Stat(filepath.Join(path, ".urb"))
	if e != nil || !st.IsDir() {
		return v, errors.New("folder is not an Urbit pier")
	}
	for _, name := range []string{"chk/image.bin", "chk/north.bin", "chk/south.bin"} {
		if st, e := os.Stat(filepath.Join(path, ".urb", name)); e == nil && st.Mode().IsRegular() {
			v.HasSnapshot = true
		}
	}
	for _, name := range []string{"log", "put"} {
		if st, e := os.Stat(filepath.Join(path, ".urb", name)); e == nil && st.IsDir() {
			v.HasEvents = true
		}
	}
	if !v.HasSnapshot || !v.HasEvents {
		return v, fmt.Errorf("pier is missing snapshots or event storage")
	}
	return v, nil
}
