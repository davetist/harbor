package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"harbor/internal/pier"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const MaxExtractedBytes int64 = 64 << 30
const maxFiles = 100000
const manifestName = "harbor-manifest.json"

// LMDB lock files contain process/reader transaction state, not ship data.
// Reusing them in a different environment can select an invalid transaction.
// Vere recreates them when it opens the restored databases.
func runtimeLock(name string) bool {
	if name == ".urb/log/lock.mdb" {
		return true
	}
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != ".urb" || parts[1] != "log" || parts[3] != "lock.mdb" || !strings.HasPrefix(parts[2], "0i") || len(parts[2]) == 2 {
		return false
	}
	for _, digit := range parts[2][2:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

type File struct {
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`
	Size     int64     `json:"size"`
	SHA256   string    `json:"sha256,omitempty"`
	Link     string    `json:"link,omitempty"`
	Mode     uint32    `json:"mode"`
	Modified time.Time `json:"modified"`
}
type Manifest struct {
	Version        int       `json:"version"`
	ShipIdentifier string    `json:"shipIdentifier"`
	Platform       string    `json:"platform"`
	VereVersion    string    `json:"vereVersion"`
	LoomExponent   int       `json:"loomExponent"`
	CreatedAt      time.Time `json:"createdAt"`
	Files          []File    `json:"files"`
}
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}
func inside(root, p string) bool {
	relative, e := filepath.Rel(root, p)
	return e == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}
func Export(ctx context.Context, source, destination string) (Manifest, error) {
	return ExportWithMetadata(ctx, source, destination, Manifest{ShipIdentifier: filepath.Base(source), Platform: runtime.GOOS + "/" + runtime.GOARCH})
}
func ExportWithMetadata(ctx context.Context, source, destination string, m Manifest) (Manifest, error) {
	real, e := filepath.EvalSymlinks(source)
	if e != nil {
		return m, e
	}
	if e = pier.VerifyRegistered(real, nil, false); e != nil {
		return m, e
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(destination))
	if e != nil {
		return m, e
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	if inside(real, destination) {
		return m, errors.New("export must be outside the source pier")
	}
	if _, e = os.Lstat(destination); !os.IsNotExist(e) {
		return m, errors.New("export destination already exists")
	}
	if locked, e := pier.WriterLocked(real); e != nil || locked {
		return m, errors.New("stop the ship before exporting the pier")
	}
	release, e := pier.Guard(real)
	if e != nil {
		return m, e
	}
	defer release()
	unblock, e := pier.ExcludeWriter(real)
	if e != nil {
		return m, e
	}
	defer unblock()
	f, e := os.CreateTemp(parent, ".harbor-export-")
	if e != nil {
		return m, e
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if e = f.Chmod(0600); e != nil {
		return m, e
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	m.Version = 1
	m.CreatedAt = time.Now().UTC()
	m.Files = []File{}
	var total int64
	e = filepath.Walk(real, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if p == real {
			return nil
		}
		rel, e := filepath.Rel(real, p)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if rel == ".vere.lock" || rel == ".urb/conn.sock" || rel == ".http.ports" {
			return nil
		}
		if info.Mode().IsRegular() && runtimeLock(rel) {
			return nil
		}
		if info.Mode()&os.ModeSocket != 0 {
			return errors.New("unexpected socket in pier")
		}
		if len(m.Files) >= maxFiles {
			return errors.New("pier file count exceeds limit")
		}
		v := File{Path: rel, Mode: uint32(info.Mode().Perm()), Modified: info.ModTime()}
		h, e := tar.FileInfoHeader(info, "")
		if e != nil {
			return e
		}
		h.Name = "pier/" + rel
		switch {
		case info.IsDir():
			v.Kind = "directory"
		case info.Mode().IsRegular():
			v.Kind = "file"
			v.Size = info.Size()
			total += v.Size
			if total > MaxExtractedBytes {
				return errors.New("pier exceeds full-export size limit")
			}
		case info.Mode()&os.ModeSymlink != 0:
			v.Kind = "symlink"
			link, e := os.Readlink(p)
			if e != nil {
				return e
			}
			target, e := filepath.EvalSymlinks(p)
			if e != nil || !inside(real, target) || filepath.IsAbs(link) {
				return errors.New("symlink escapes pier or has an unavailable target")
			}
			v.Link = filepath.ToSlash(link)
			h.Linkname = v.Link
		default:
			return errors.New("unsupported special file in pier")
		}
		if e = tw.WriteHeader(h); e != nil {
			return e
		}
		if v.Kind == "file" {
			input, e := os.Open(p)
			if e != nil {
				return e
			}
			hash := sha256.New()
			n, e := io.Copy(io.MultiWriter(tw, hash), contextReader{ctx, input})
			input.Close()
			if e != nil {
				return e
			}
			if n != v.Size {
				return errors.New("source changed during export")
			}
			v.SHA256 = hex.EncodeToString(hash.Sum(nil))
		}
		m.Files = append(m.Files, v)
		return nil
	})
	if e != nil {
		tw.Close()
		gz.Close()
		return m, e
	}
	b, e := json.Marshal(m)
	if e != nil {
		return m, e
	}
	if len(b) > 32<<20 {
		return m, errors.New("archive manifest exceeds limit")
	}
	if e = tw.WriteHeader(&tar.Header{Name: manifestName, Mode: 0600, Size: int64(len(b)), Typeflag: tar.TypeReg}); e != nil {
		return m, e
	}
	if _, e = tw.Write(b); e != nil {
		return m, e
	}
	if e = tw.Close(); e != nil {
		return m, e
	}
	if e = gz.Close(); e != nil {
		return m, e
	}
	if e = f.Sync(); e != nil {
		return m, e
	}
	if e = f.Close(); e != nil {
		return m, e
	}
	if e = os.Link(f.Name(), destination); e != nil {
		return m, e
	}
	return m, nil
}
func safeEntry(name string) (string, error) {
	if !strings.HasPrefix(name, "pier/") {
		return "", errors.New("archive entry is outside pier root")
	}
	p := strings.TrimPrefix(name, "pier/")
	if p == "" || path.Clean(p) != p || path.IsAbs(p) || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "\\") || strings.ContainsRune(p, 0) {
		return "", errors.New("invalid archive path")
	}
	return p, nil
}
func scan(ctx context.Context, archive string, visit func(File, io.Reader) error) (Manifest, error) {
	var m Manifest
	f, e := os.Open(archive)
	if e != nil {
		return m, e
	}
	defer f.Close()
	gz, e := gzip.NewReader(contextReader{ctx, f})
	if e != nil {
		return m, e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	observed := make(map[string]File)
	var total int64
	found := false
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return m, e
		}
		if e = ctx.Err(); e != nil {
			return m, e
		}
		if h.Name == manifestName {
			if found || h.Typeflag != tar.TypeReg || h.Size > 32<<20 {
				return m, errors.New("invalid archive manifest")
			}
			d := json.NewDecoder(io.LimitReader(tr, 32<<20+1))
			d.DisallowUnknownFields()
			if e = d.Decode(&m); e != nil {
				return m, errors.New("invalid archive manifest JSON")
			}
			found = true
			continue
		}
		if found {
			return m, errors.New("archive manifest must be the final entry")
		}
		name, e := safeEntry(h.Name)
		if e != nil {
			return m, e
		}
		if _, ok := observed[name]; ok {
			return m, errors.New("duplicate archive entry")
		}
		if len(observed) >= maxFiles {
			return m, errors.New("too many archive entries")
		}
		v := File{Path: name, Size: h.Size, Mode: uint32(h.Mode) & 0777, Modified: h.ModTime}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if h.Size < 0 || h.Size > MaxExtractedBytes {
				return m, errors.New("archive file exceeds size limit")
			}
			total += h.Size
			if total > MaxExtractedBytes {
				return m, errors.New("archive exceeds extracted-size limit")
			}
			v.Kind = "file"
			hash := sha256.New()
			reader := io.TeeReader(contextReader{ctx, tr}, hash)
			if visit != nil {
				if e = visit(v, reader); e != nil {
					return m, e
				}
			} else {
				if _, e = io.Copy(io.Discard, reader); e != nil {
					return m, e
				}
			}
			if _, e = io.Copy(io.Discard, reader); e != nil {
				return m, e
			}
			v.SHA256 = hex.EncodeToString(hash.Sum(nil))
		case tar.TypeDir:
			v.Kind = "directory"
			v.Size = 0
			if visit != nil {
				if e = visit(v, nil); e != nil {
					return m, e
				}
			}
		case tar.TypeSymlink:
			v.Kind = "symlink"
			v.Size = 0
			v.Link = h.Linkname
			target := path.Clean(path.Join(path.Dir(name), h.Linkname))
			if path.IsAbs(h.Linkname) || strings.Contains(h.Linkname, "\\") || target == ".." || strings.HasPrefix(target, "../") {
				return m, errors.New("symlink escapes archive")
			}
			if visit != nil {
				if e = visit(v, nil); e != nil {
					return m, e
				}
			}
		default:
			return m, errors.New("archive contains an unsupported special file")
		}
		observed[name] = v
	}
	if e = validateManifest(ctx, m, observed); e != nil {
		return m, e
	}
	return m, nil
}

func validateManifest(ctx context.Context, m Manifest, observed map[string]File) error {
	if m.Version != 1 || len(m.Files) != len(observed) {
		return errors.New("archive integrity manifest missing or incomplete")
	}
	seen := make(map[string]bool)
	// Cache only complete, valid suffixes. Every observed node is traversed
	// once, even when many links share a long chain; keep this iterative.
	resolved := make(map[string]bool)
	for _, expected := range m.Files {
		if e := ctx.Err(); e != nil {
			return e
		}
		v, ok := observed[expected.Path]
		if !ok || seen[expected.Path] || v.Kind != expected.Kind || v.Size != expected.Size || v.Link != expected.Link || v.SHA256 != expected.SHA256 || v.Mode != expected.Mode {
			return fmt.Errorf("archive integrity check failed for %q", expected.Path)
		}
		seen[expected.Path] = true
		for parent := path.Dir(expected.Path); parent != "."; parent = path.Dir(parent) {
			if e := ctx.Err(); e != nil {
				return e
			}
			if observed[parent].Kind == "symlink" {
				return errors.New("archive writes through a symlink")
			}
		}
		if v.Kind == "symlink" && !resolved[v.Path] {
			target := v.Path
			visited := make(map[string]bool)
			for {
				if e := ctx.Err(); e != nil {
					return e
				}
				if resolved[target] {
					break
				}
				if visited[target] {
					return errors.New("cyclic archive symlink")
				}
				visited[target] = true
				entry, ok := observed[target]
				if !ok {
					return errors.New("symlink target missing from archive")
				}
				if entry.Kind != "symlink" {
					break
				}
				target = path.Clean(path.Join(path.Dir(target), entry.Link))
			}
			for target := range visited {
				if e := ctx.Err(); e != nil {
					return e
				}
				resolved[target] = true
			}
		}
	}
	return nil
}
func Validate(ctx context.Context, archive string) (Manifest, error) { return scan(ctx, archive, nil) }
func Restore(ctx context.Context, archive, destination string) (string, error) {
	m, e := Validate(ctx, archive)
	if e != nil {
		return "", e
	}
	if _, e = os.Lstat(destination); !os.IsNotExist(e) {
		return "", errors.New("restore destination already exists")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(destination))
	if e != nil {
		return "", e
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	var total uint64
	for _, v := range m.Files {
		if v.Kind == "file" {
			total += uint64(v.Size)
		}
	}
	var fs unix.Statfs_t
	if e = unix.Statfs(parent, &fs); e != nil {
		return "", e
	}
	if total > uint64(fs.Bavail)*uint64(fs.Bsize) {
		return "", errors.New("not enough free storage to restore full pier")
	}
	stage, e := os.MkdirTemp(parent, ".harbor-restore-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(stage)
	var links []File
	_, e = scan(ctx, archive, func(v File, r io.Reader) error {
		// Older valid archives may contain runtime locks. scan still consumes
		// and verifies their bytes against the manifest, without restoring them.
		if v.Kind == "file" && runtimeLock(v.Path) {
			return nil
		}
		p := filepath.Join(stage, filepath.FromSlash(v.Path))
		if !inside(stage, p) {
			return errors.New("invalid restore path")
		}
		if v.Kind == "directory" {
			return os.MkdirAll(p, 0700)
		}
		if v.Kind == "symlink" {
			links = append(links, v)
			return nil
		}
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return e
		}
		f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = io.Copy(f, contextReader{ctx, r})
		if e == nil {
			e = f.Sync()
		}
		f.Close()
		if e != nil {
			return e
		}
		if e = os.Chmod(p, os.FileMode(v.Mode)); e != nil {
			return e
		}
		return os.Chtimes(p, v.Modified, v.Modified)
	})
	if e != nil {
		return "", e
	}
	for _, link := range links {
		p := filepath.Join(stage, filepath.FromSlash(link.Path))
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return "", e
		}
		if e = os.Symlink(filepath.FromSlash(link.Link), p); e != nil {
			return "", e
		}
	}
	// Publish without replacing any file or directory created since preflight.
	if e = renameNoReplace(stage, destination); e != nil {
		return "", e
	}
	return destination, nil
}
