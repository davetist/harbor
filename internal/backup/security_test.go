package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"harbor/internal/pier"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func securityArchive(t *testing.T, files []File, manifest []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fixture.tgz")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, v := range files {
		kind := byte(tar.TypeReg)
		if v.Kind == "directory" {
			kind = tar.TypeDir
		}
		if v.Kind == "symlink" {
			kind = tar.TypeSymlink
		}
		if err := tw.WriteHeader(&tar.Header{Name: "pier/" + v.Path, Typeflag: kind, Linkname: v.Link, Mode: int64(v.Mode)}); err != nil {
			t.Fatal(err)
		}
	}
	if manifest != nil {
		if err := tw.WriteHeader(&tar.Header{Name: manifestName, Typeflag: tar.TypeReg, Size: int64(len(manifest))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(manifest); err != nil {
			t.Fatal(err)
		}
	}
	for _, close := range []func() error{tw.Close, gz.Close, f.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestSecurityArchiveRejectsComponentExpansionEscape(t *testing.T) {
	files := []File{
		{Path: "safe", Kind: "directory"}, {Path: "safe/deep", Kind: "directory"},
		{Path: "safe/outside", Kind: "file"},
		{Path: "safe/deep/up", Kind: "symlink", Link: ".."},
		{Path: "escape", Kind: "symlink", Link: "safe/deep/up/../../outside"},
	}
	m, observed := manifestGraph(files)
	if err := validateManifest(context.Background(), m, observed); err == nil {
		t.Fatal("component expansion escaped the pier")
	}
}

func TestSecurityArchiveRejectsCaseAliasedLinkParents(t *testing.T) {
	m, observed := manifestGraph([]File{{Path: "safe", Kind: "directory"}, {Path: "jump", Kind: "symlink", Link: "safe"}, {Path: "JUMP/new-link", Kind: "symlink", Link: "../safe"}})
	if err := validateManifest(context.Background(), m, observed); err == nil {
		t.Fatal("case-aliased symlink ancestor accepted")
	}
}

func TestSecurityArchiveRejectsPathsBeforeManifest(t *testing.T) {
	for _, name := range []string{strings.Repeat("x", 4097), strings.Repeat("a/", 129) + "leaf", strings.Repeat("x", 256) + "/leaf"} {
		p := securityArchive(t, []File{{Path: name, Kind: "file"}}, nil)
		if _, err := Validate(context.Background(), p); err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("path was not bounded before manifest decoding: %v", err)
		}
	}
}

func TestSecurityArchiveBoundsManifestBeforeDecodingExtraFiles(t *testing.T) {
	p := securityArchive(t, nil, []byte(`{"version":1,"files":[{},INVALID`))
	if _, err := Validate(context.Background(), p); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("manifest array was decoded beyond its observed-file budget: %v", err)
	}
}

func TestSecurityArchiveCountsImplicitDirectories(t *testing.T) {
	var files []File
	for i := 0; i < 1000; i++ {
		files = append(files, File{Path: fmt.Sprintf("tree-%04d/", i) + strings.Repeat("a/", 110) + "leaf", Kind: "file"})
	}
	m, observed := manifestGraph(files)
	if err := validateManifest(context.Background(), m, observed); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("uncounted implicit directories accepted: %v", err)
	}
}

func TestSecurityArchiveRejectsRuntimeLockLinks(t *testing.T) {
	for _, name := range []string{".vere.lock", ".urb/log/lock.mdb", ".urb/log/0i123/lock.mdb"} {
		files := []File{{Path: "safe", Kind: "directory"}, {Path: name, Kind: "symlink", Link: strings.Repeat("../", strings.Count(name, "/")) + "safe"}}
		m, _ := manifestGraph(files)
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		p := securityArchive(t, files, b)
		if _, err := Validate(context.Background(), p); err == nil {
			t.Fatalf("native lock symlink accepted: %s", name)
		}
	}
}

type swappedStageContext struct {
	context.Context
	parent, sibling string
	swapped         bool
}

func (c *swappedStageContext) Err() error {
	if c.swapped {
		return context.Canceled
	}
	stages, _ := filepath.Glob(filepath.Join(c.parent, ".harbor-restore-*"))
	if len(stages) == 0 {
		return nil
	}
	if os.Rename(stages[0], stages[0]+"-moved") != nil {
		return nil
	}
	if os.Rename(c.sibling, stages[0]) != nil {
		return nil
	}
	c.sibling = stages[0]
	c.swapped = true
	return context.Canceled
}

func TestSecurityRestoreCleanupPreservesReplacementDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "data"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "archive.tgz")
	if _, err := Export(context.Background(), source, archive); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(root, "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "secret"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &swappedStageContext{Context: context.Background(), parent: root, sibling: private}
	if _, err := restore(ctx, archive, filepath.Join(root, "restored")); err == nil || !ctx.swapped {
		t.Fatalf("replacement trial did not run: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(ctx.sibling, "secret")); err != nil || string(b) != "keep" {
		t.Fatalf("cleanup removed private sibling contents: %v", err)
	}
}

func TestSecurityArchiveBoundsHiddenExtensionRecords(t *testing.T) {
	// GNU extension records are consumed internally by tar.Next. Hundreds of
	// these produce just one logical entry, bypassing a logical file counter.
	var header bytes.Buffer
	tw := tar.NewWriter(&header)
	const extensionSize = 512 << 10
	if err := tw.WriteHeader(&tar.Header{Name: "././@LongLink", Typeflag: tar.TypeReg, Size: extensionSize, Mode: 0600, Format: tar.FormatGNU}); err != nil {
		t.Fatal(err)
	}
	raw := header.Bytes()
	raw[156] = tar.TypeGNULongName
	for i := 148; i < 156; i++ {
		raw[i] = ' '
	}
	sum := 0
	for _, b := range raw {
		sum += int(b)
	}
	copy(raw[148:156], fmt.Sprintf("%06o\x00 ", sum))
	longName := make([]byte, extensionSize)
	copy(longName, "pier/leaf")
	p := filepath.Join(t.TempDir(), "hidden.tgz")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	for i := 0; i < maxStreamMetadata/extensionSize+1; i++ {
		if _, err := gz.Write(raw); err != nil {
			t.Fatal(err)
		}
		if _, err := gz.Write(longName); err != nil {
			t.Fatal(err)
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(context.Background(), p); err == nil || !strings.Contains(err.Error(), "metadata or stream limit") {
		t.Fatalf("hidden extension bytes bypassed physical budget: %v", err)
	}
}

func TestSecurityArchiveBudgetsRetainedNamesAndAllImplicitObjects(t *testing.T) {
	// Each entry is individually valid. The aggregate metadata or materialized
	// ancestor count must stop the stream before any missing-manifest error.
	var files []File
	for i := 0; i < 1000; i++ {
		files = append(files, File{Path: fmt.Sprintf("tree-%04d/", i) + strings.Repeat("a/", 110) + "leaf", Kind: "file"})
	}
	p := securityArchive(t, files, nil)
	if _, err := Validate(context.Background(), p); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("stream did not budget implicit objects: %v", err)
	}
	// Long, shallow names hit the retained-name budget without hitting depth or
	// file-count limits. Reuse one backing string, as a PAX header may do.
	b := newObjectBudget()
	prefix := strings.Repeat(strings.Repeat("x", 250)+"/", 14)
	for i := 0; ; i++ {
		name := prefix + fmt.Sprintf("leaf-%05d", i)
		err := b.add(File{Path: name, Kind: "file"})
		if err != nil {
			if !strings.Contains(err.Error(), "metadata limit") || i >= maxFiles {
				t.Fatal(err)
			}
			break
		}
	}
}

func TestSecuritySymlinkTraversalRequiresDirectoryComponents(t *testing.T) {
	for _, target := range []string{"leaf/", "leaf/.", "leaf/../leaf"} {
		m, observed := manifestGraph([]File{{Path: "leaf", Kind: "file"}, {Path: "link", Kind: "symlink", Link: target}})
		if err := validateManifest(context.Background(), m, observed); err == nil {
			t.Fatalf("non-directory component traversed: %s", target)
		}
	}
}

func TestSecurityLockAliasCannotReleaseAnExistingReadHold(t *testing.T) {
	p := t.TempDir()
	release, err := pier.ExcludeWriter(p)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := os.Link(filepath.Join(p, ".vere.lock"), filepath.Join(p, "snapshot")); err != nil {
		t.Fatal(err)
	}
	if _, err := pier.WriterLocked(p); err == nil {
		t.Fatal("observer accepted a newly aliased lock")
	}
	// Opening and closing the rejected inode would silently drop the process's
	// POSIX read lock. Check actual exclusion from a separate process.
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestReviewBackupWriterHelper$")
	child.Env = append(os.Environ(), "HARBOR_REVIEW_LOCK="+filepath.Join(p, ".vere.lock"))
	if out, err := child.Output(); err != nil || string(out) != "writer blocked\n" {
		t.Fatalf("alias observation released the read hold: %s %v", out, err)
	}
}

type exportCaseLockContext struct {
	context.Context
	checks int
	probe  func()
}

func (c *exportCaseLockContext) Err() error {
	c.checks++
	if c.checks == 5 {
		c.probe()
	}
	return nil
}

func TestSecurityExportCaseAliasedLockKeepsWriterExcluded(t *testing.T) {
	p := t.TempDir()
	lock := filepath.Join(p, ".VERE.LOCK")
	if err := os.WriteFile(lock, []byte("0"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(p, ".vere.lock")); os.IsNotExist(err) {
		t.Skip("fixture requires a case-insensitive filesystem")
	}
	if err := os.WriteFile(filepath.Join(p, "data"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	probed := false
	ctx := &exportCaseLockContext{Context: context.Background(), probe: func() {
		probed = true
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(binary, "-test.run=^TestReviewBackupWriterHelper$")
		child.Env = append(os.Environ(), "HARBOR_REVIEW_LOCK="+lock)
		if out, err := child.Output(); err != nil || string(out) != "writer blocked\n" {
			t.Fatalf("export released exclusion after reading case-aliased lock: %s %v", out, err)
		}
	}}
	if _, err := Export(ctx, p, filepath.Join(t.TempDir(), "export.tgz")); err != nil {
		t.Fatal(err)
	}
	if !probed {
		t.Fatal("separate-process exclusion probe did not execute")
	}
}

func TestSecurityManifestRejectsAmbiguousAndExcessJSON(t *testing.T) {
	for _, input := range []string{
		`{"version":1,"version":1,"files":[]}`,
		`{"version":1,"unexpected":{},"files":[]}`,
		`{"version":1,"files":[]} {}`,
		`{"version":1,"files":[{"unknown":true}]}`,
		`{"version":1,"files":[{},INVALID`,
	} {
		if _, err := decodeManifest(strings.NewReader(input), 1); err == nil {
			t.Fatalf("ambiguous or excess manifest accepted: %s", input)
		}
	}
	if _, err := decodeManifest(strings.NewReader(`{"version":1,"files":[]}`), 0); err != nil {
		t.Fatal("empty valid manifest rejected", err)
	}
}

type countedManifestReader struct {
	io.Reader
	bytes int
}

func (r *countedManifestReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytes += n
	return n, err
}

func TestSecurityManifestRejectsTrailingJSONBeforeExpansion(t *testing.T) {
	r := &countedManifestReader{Reader: strings.NewReader(`{"version":1,"files":[]} [` + strings.Repeat(`{},`, 8192) + `{}]`)}
	if _, err := decodeManifest(r, 0); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if r.bytes > 1024 {
		t.Fatalf("trailing objects were decoded before rejection: read %d bytes", r.bytes)
	}
}

func TestSecurityRestoreCapacityIncludesEmptyObjectsAndInodes(t *testing.T) {
	m := Manifest{Files: []File{{Path: "a/b/empty", Kind: "file"}}}
	stat := unix.Statfs_t{Bsize: 4096, Bavail: (64 << 20) / 4096, Files: 1000, Ffree: 1000}
	if err := restoreCapacity(m, stat); err == nil {
		t.Fatal("empty files and implicit directories were treated as zero storage")
	}
	stat.Bavail *= 2
	stat.Ffree = 3
	if err := restoreCapacity(m, stat); err == nil {
		t.Fatal("insufficient inode budget accepted")
	}
	stat.Ffree = 1000
	if err := restoreCapacity(m, stat); err != nil {
		t.Fatal("ordinary small archive rejected", err)
	}
}

func TestSecurityRestoreRejectsActualFilesystemLinkAliases(t *testing.T) {
	dir := t.TempDir()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, pair := range [][2]string{{"jump", "JUMP"}, {"caf\u00e9", "cafe\u0301"}} {
		if err := r.Symlink(".", pair[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Lstat(pair[1]); err == nil {
			if err := restoreDirectories(r, pair[1]+"/child"); err == nil {
				t.Fatal("creation through a filesystem link alias accepted")
			}
			if _, err := r.Lstat("child"); !os.IsNotExist(err) {
				t.Fatal("alias caused unexpected directory creation", err)
			}
		}
		if err := r.Remove(pair[0]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSecurityArchiveHonorsExpiredDeadlineBeforeWork(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	p := securityArchive(t, nil, nil)
	if _, err := Validate(ctx, p); err != context.DeadlineExceeded {
		t.Fatalf("expired parsing deadline ignored: %v", err)
	}
	// Header quotas must report failure rather than pretending truncated input
	// is an ordinary archive EOF.
	r := &archiveReader{ctx: context.Background(), r: strings.NewReader("x"), metadata: maxStreamMetadata, header: true}
	if _, err := io.ReadAll(r); err == nil || err == io.EOF {
		t.Fatalf("exhausted metadata became successful EOF: %v", err)
	}
}
