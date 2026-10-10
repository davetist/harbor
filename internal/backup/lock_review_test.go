package backup

import (
	"bufio"
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"harbor/internal/pier"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

type reviewContext struct {
	context.Context
	once sync.Once
	f    func()
}

func (c *reviewContext) Err() error { c.once.Do(c.f); return nil }
func TestReviewBackupWriterHelper(t *testing.T) {
	p := os.Getenv("HARBOR_REVIEW_LOCK")
	if p == "" {
		return
	}
	f, e := os.OpenFile(p, os.O_RDWR, 0600)
	if e != nil {
		os.Exit(2)
	}
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 1}
	if unix.FcntlFlock(f.Fd(), unix.F_SETLK, &lock) != nil {
		fmt.Println("writer blocked")
		os.Exit(0)
	}
	fmt.Println("writer acquired")
	os.Stdin.Read(make([]byte, 1))
	f.Close()
	os.Exit(0)
}
func TestReviewBackupExcludesExternalWriter(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0700)
	os.WriteFile(filepath.Join(source, ".vere.lock"), []byte("0"), 0600)
	os.WriteFile(filepath.Join(source, "data"), []byte("synthetic data"), 0600)
	var child *exec.Cmd
	ctx := &reviewContext{Context: context.Background(), f: func() {
		binary, _ := os.Executable()
		child = exec.Command(binary, "-test.run=^TestReviewBackupWriterHelper$")
		child.Env = append(os.Environ(), "HARBOR_REVIEW_LOCK="+filepath.Join(source, ".vere.lock"))
		out, _ := child.StdoutPipe()
		in, _ := child.StdinPipe()
		if e := child.Start(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { in.Close(); child.Wait() })
		line, _ := bufio.NewReader(out).ReadString('\n')
		if line != "writer blocked\n" {
			t.Fatalf("external writer entered during export: %q", line)
		}
	}}
	archive := filepath.Join(root, "backup.tgz")
	_, err := Export(ctx, source, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Validate(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
}

func TestBackupLockObservationCannotReleaseExclusion(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.Mkdir(source, 0700)
	os.WriteFile(filepath.Join(source, ".vere.lock"), []byte("0"), 0600)
	release, err := pier.ExcludeWriter(source)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if locked, err := pier.WriterLocked(source); err != nil || !locked {
		t.Fatal("reader not recognized", err)
	}
	binary, _ := os.Executable()
	child := exec.Command(binary, "-test.run=^TestReviewBackupWriterHelper$")
	child.Env = append(os.Environ(), "HARBOR_REVIEW_LOCK="+filepath.Join(source, ".vere.lock"))
	out, err := child.Output()
	if err != nil || string(out) != "writer blocked\n" {
		t.Fatal("observation released export lock", err)
	}
}
