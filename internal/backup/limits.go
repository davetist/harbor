package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

const (
	ArchiveTimeout      = 30 * time.Minute
	maxPathBytes        = 4096
	maxPathDepth        = 128
	maxRetainedMetadata = 32 << 20
	maxStreamMetadata   = 128 << 20
	maxManifestBytes    = 32 << 20
)

// Metadata is charged while Next consumes physical headers, extension records
// and padding, not merely when it returns a logical entry. Payload has its own
// larger budget. Exhaustion is an error, never a synthetic successful EOF.
type archiveReader struct {
	ctx             context.Context
	r               io.Reader
	total, metadata int64
	header          bool
}

// Exports obey the same physical budgets as imports, including the PAX records
// emitted by archive/tar. Do not publish an archive Harbor cannot restore.
type archiveWriter struct {
	w               io.Writer
	total, metadata int64
	header          bool
}

func (w *archiveWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > MaxExtractedBytes+maxStreamMetadata-w.total || (w.header && int64(len(p)) > maxStreamMetadata-w.metadata) {
		return 0, errors.New("archive decompressed metadata or stream limit exceeded")
	}
	n, err := w.w.Write(p)
	w.total += int64(n)
	if w.header {
		w.metadata += int64(n)
	}
	return n, err
}

func (r *archiveReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	remaining := int64(MaxExtractedBytes+maxStreamMetadata) - r.total
	if r.header {
		remaining = min(remaining, int64(maxStreamMetadata)-r.metadata)
	}
	if remaining <= 0 {
		return 0, errors.New("archive decompressed metadata or stream limit exceeded")
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.r.Read(p)
	r.total += int64(n)
	if r.header {
		r.metadata += int64(n)
	}
	return n, err
}

func boundedPath(p string) error {
	if len(p) > maxPathBytes || strings.Count(p, "/")+1 > maxPathDepth {
		return errors.New("archive path or link limit exceeded")
	}
	for _, part := range strings.Split(p, "/") {
		if len(part) > 255 {
			return errors.New("archive component length limit exceeded")
		}
	}
	return nil
}

// Decode the files array one object at a time. Reject excess elements before
// decoding/allocating them, even when the remainder is malformed JSON.
func decodeManifest(r io.Reader, observed int) (Manifest, error) {
	var m Manifest
	d := json.NewDecoder(io.LimitReader(r, maxManifestBytes+1))
	d.DisallowUnknownFields()
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return m, errors.New("invalid archive manifest JSON")
	}
	seen := map[string]bool{}
	metadata := 0
	for d.More() {
		t, err = d.Token()
		if err != nil {
			return m, err
		}
		key, ok := t.(string)
		if !ok || seen[key] {
			return m, errors.New("duplicate or invalid archive manifest field")
		}
		seen[key] = true
		switch key {
		case "version":
			err = d.Decode(&m.Version)
			if err == nil && m.Version != 1 {
				return m, errors.New("unsupported archive manifest version")
			}
		case "shipIdentifier":
			err = d.Decode(&m.ShipIdentifier)
		case "platform":
			err = d.Decode(&m.Platform)
		case "vereVersion":
			err = d.Decode(&m.VereVersion)
		case "loomExponent":
			err = d.Decode(&m.LoomExponent)
		case "createdAt":
			err = d.Decode(&m.CreatedAt)
		case "files":
			t, err = d.Token()
			if err != nil || t != json.Delim('[') {
				return m, errors.New("invalid archive files array")
			}
			for d.More() {
				if len(m.Files) >= maxFiles || len(m.Files) >= observed {
					return m, errors.New("archive manifest file count limit exceeded")
				}
				var v File
				if err = d.Decode(&v); err != nil {
					return m, err
				}
				if err = boundedPath(v.Path); err != nil {
					return m, err
				}
				if err = boundedPath(v.Link); err != nil {
					return m, err
				}
				metadata += len(v.Path) + len(v.Link)
				if metadata > maxRetainedMetadata {
					return m, errors.New("archive manifest metadata limit exceeded")
				}
				m.Files = append(m.Files, v)
			}
			_, err = d.Token()
		default:
			return m, errors.New("unknown archive manifest field")
		}
		if err != nil {
			return m, err
		}
		if len(m.ShipIdentifier) > 1024 || len(m.Platform) > 1024 || len(m.VereVersion) > 1024 {
			return m, errors.New("archive manifest field limit exceeded")
		}
	}
	if _, err = d.Token(); err != nil {
		return m, err
	}
	// Inspect just the next token. Decoding arbitrary trailing JSON into any
	// would allocate a second, unbounded object graph before rejecting it.
	if _, err = d.Token(); err != io.EOF {
		return m, errors.New("trailing archive manifest JSON")
	}
	return m, nil
}

// Count all materialized objects, including implicit directories, before any
// extraction. Cache ancestors so shared prefixes are traversed only once.
type objectBudget struct {
	objects map[string]File
	bytes   int
}

func newObjectBudget() *objectBudget {
	return &objectBudget{objects: map[string]File{".": {Path: ".", Kind: "directory"}}}
}
func (b *objectBudget) add(v File) error {
	if err := boundedPath(v.Path); err != nil {
		return err
	}
	if old, ok := b.objects[v.Path]; ok {
		if old.Kind != "directory" || v.Kind != "directory" {
			return errors.New("conflicting archive object")
		}
	} else {
		b.bytes += len(v.Path)
	}
	b.objects[v.Path] = v
	for parent := path.Dir(v.Path); parent != "."; parent = path.Dir(parent) {
		if old, ok := b.objects[parent]; ok {
			if old.Kind != "directory" {
				return errors.New("archive writes through a symlink")
			}
			break
		}
		b.objects[parent] = File{Path: parent, Kind: "directory"}
		b.bytes += len(parent)
		if len(b.objects) > maxFiles+1 || b.bytes > maxRetainedMetadata {
			return errors.New("archive materialized object or metadata limit exceeded")
		}
	}
	if len(b.objects) > maxFiles+1 || b.bytes > maxRetainedMetadata {
		return errors.New("archive materialized object or metadata limit exceeded")
	}
	return nil
}
func archiveObjects(files []File) (map[string]File, error) {
	b := newObjectBudget()
	for _, v := range files {
		if err := b.add(v); err != nil {
			return nil, err
		}
	}
	return b.objects, nil
}

type linkFrame struct {
	name, at string
	parts    []string
	index    int
}

func resolveArchiveLink(ctx context.Context, name string, objects map[string]File, resolved map[string]string) (string, error) {
	if target, ok := resolved[name]; ok {
		return target, nil
	}
	frames := []linkFrame{{name: name, at: path.Dir(name), parts: strings.Split(objects[name].Link, "/")}}
	active := map[string]bool{name: true}
	for len(frames) > 0 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		f := &frames[len(frames)-1]
		if f.index == len(f.parts) {
			target := f.at
			resolved[f.name] = target
			delete(active, f.name)
			frames = frames[:len(frames)-1]
			if len(frames) == 0 {
				return target, nil
			}
			frames[len(frames)-1].at = target
			continue
		}
		part := f.parts[f.index]
		f.index++
		if objects[f.at].Kind != "directory" {
			return "", errors.New("archive symlink traverses a non-directory")
		}
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if f.at == "." {
				return "", errors.New("symlink escapes archive")
			}
			f.at = path.Dir(f.at)
			continue
		}
		next := path.Join(f.at, part)
		v, ok := objects[next]
		if !ok {
			return "", errors.New("symlink target missing from archive")
		}
		f.at = next
		if v.Kind == "symlink" {
			if target, ok := resolved[next]; ok {
				f.at = target
				continue
			}
			if active[next] {
				return "", errors.New("cyclic archive symlink")
			}
			active[next] = true
			frames = append(frames, linkFrame{name: next, at: path.Dir(next), parts: strings.Split(v.Link, "/")})
		}
	}
	return "", errors.New("invalid archive link")
}

func validateArchiveManifest(ctx context.Context, m Manifest, observed map[string]File) error {
	if m.Version != 1 || len(m.Files) != len(observed) {
		return errors.New("archive integrity manifest missing or incomplete")
	}
	seen := make(map[string]bool)
	// Retain exact paths for integrity; case aliases of link ancestors are
	// refused as ambiguous portable archive paths. Rooted extraction also
	// handles the destination filesystem's additional Unicode aliases.
	foldedLinks := map[string]bool{}
	checkedParents := map[string]bool{}
	for _, v := range observed {
		if v.Kind == "symlink" {
			foldedLinks[strings.ToLower(v.Path)] = true
		}
	}
	for _, v := range m.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		actual, ok := observed[v.Path]
		if !ok || seen[v.Path] || actual.Kind != v.Kind || actual.Size != v.Size || actual.Link != v.Link || actual.SHA256 != v.SHA256 || actual.Mode != v.Mode {
			return fmt.Errorf("archive integrity check failed for %q", v.Path)
		}
		seen[v.Path] = true
		if err := boundedPath(v.Path); err != nil {
			return err
		}
		if err := boundedPath(v.Link); err != nil {
			return err
		}
		for parent := path.Dir(v.Path); parent != "."; parent = path.Dir(parent) {
			if checkedParents[parent] {
				break
			}
			if foldedLinks[strings.ToLower(parent)] {
				return errors.New("archive writes through a symlink")
			}
			checkedParents[parent] = true
		}
	}
	objects, err := archiveObjects(m.Files)
	if err != nil {
		return err
	}
	resolved := map[string]string{}
	for _, v := range m.Files {
		if v.Kind == "symlink" {
			if _, err := resolveArchiveLink(ctx, v.Path, objects, resolved); err != nil {
				return err
			}
		}
	}
	return nil
}
