// Package installation owns the per-user runner and its OS service registration.
// Ship data and configuration survive upgrades and removal.
package installation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"harbor/internal/config"
	"harbor/internal/pier"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const serviceName = "org.harbor.runner"

var errNoSession = errors.New("no user service manager is running")

type Paths struct {
	Home, Root, Binary, Record, Config, Unit, Logs, Label string
}

type Manager struct {
	Paths                           Paths
	Platform, Source, UID, Username string
	Run                             func(context.Context, string, ...string) ([]byte, error)
}

type Options struct {
	Config                     string
	Upgrade, NoStart, Headless bool
	IfSource                   string
}

type record struct {
	Schema   int    `json:"schema"`
	Source   string `json:"source"`
	Config   string `json:"config"`
	Unit     string `json:"unit"`
	SHA256   string `json:"sha256"`
	Enabled  bool   `json:"enabled"`
	Headless bool   `json:"headless"`
}

func locations(platform, home, configHome string) Paths {
	p := Paths{Home: home, Config: filepath.Join(configHome, "Harbor", "config.json"), Label: serviceName}
	if platform == "darwin" {
		p.Root = filepath.Join(home, "Library", "Application Support", "Harbor", "runner")
		p.Unit = filepath.Join(home, "Library", "LaunchAgents", serviceName+".plist")
		p.Logs = filepath.Join(home, "Library", "Logs", "Harbor")
	} else {
		// A stable record location lets a package manager find its own installations,
		// even when the desktop session uses a custom XDG_CONFIG_HOME.
		p.Root = filepath.Join(home, ".local", "share", "Harbor", "runner")
		p.Label = "harbor"
		p.Unit = filepath.Join(configHome, "systemd", "user", "harbor.service")
	}
	p.Binary = filepath.Join(p.Root, "harbor")
	p.Record = filepath.Join(p.Root, "installation.json")
	return p
}

func New() (*Manager, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil, errors.New("installation supports macOS and Linux")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	current, err := user.Current()
	if err != nil {
		return nil, err
	}
	source, err := os.Executable()
	if err != nil {
		return nil, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return nil, err
	}
	m := &Manager{Paths: locations(runtime.GOOS, home, base), Platform: runtime.GOOS, Source: source, UID: current.Uid, Username: current.Username}
	m.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if runtime.GOOS == "linux" && name == "systemctl" {
			bus := "/run/user/" + m.UID + "/bus"
			if directory := os.Getenv("XDG_RUNTIME_DIR"); directory != "" {
				bus = filepath.Join(directory, "bus")
			}
			if address := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); strings.HasPrefix(address, "unix:path=") {
				bus = strings.Split(strings.TrimPrefix(address, "unix:path="), ",")[0]
			}
			if _, err := os.Stat(bus); os.IsNotExist(err) {
				return nil, errNoSession
			}
		}
		cmd := exec.CommandContext(ctx, name, args...)
		if name == "sudo" {
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return nil, cmd.Run()
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("%s failed: %w", name, err)
		}
		return out, nil
	}
	return m, nil
}

func regular(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256<<20 {
		return nil, fmt.Errorf("not an ordinary installation file: %s", path)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink != 1 {
		return nil, fmt.Errorf("installation file has hard links: %s", path)
	}
	return io.ReadAll(io.LimitReader(f, 256<<20))
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func privateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("installation directory must not be a symlink: %s", path)
	}
	return nil
}

func atomicFile(path string, contents []byte, mode os.FileMode) error {
	if _, err := os.Lstat(path); err == nil {
		if _, err = regular(path); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := privateDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".harbor-install-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(contents)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (m *Manager) lock(ctx context.Context) (func(), error) {
	if err := privateDir(m.Paths.Root); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(m.Paths.Root, ".install.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Manager) readRecord() (record, error) {
	var r record
	b, err := regular(m.Paths.Record)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.Schema != 1 || !filepath.IsAbs(r.Config) || !filepath.IsAbs(r.Source) || !filepath.IsAbs(r.Unit) || len(r.SHA256) != 64 {
		return r, errors.New("unrecognized Harbor installation record; preserve it for inspection")
	}
	if filepath.Base(r.Unit) != filepath.Base(m.Paths.Unit) {
		return r, errors.New("unrecognized Harbor service registration")
	}
	m.Paths.Unit = r.Unit
	return r, nil
}

func (m *Manager) save(r record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicFile(m.Paths.Record, append(b, '\n'), 0600)
}

func (m *Manager) owned(r record) error {
	b, err := regular(m.Paths.Binary)
	if err != nil {
		return err
	}
	if digest(b) != r.SHA256 {
		return errors.New("installed runner changed outside Harbor; preserve it before replacing or removing it")
	}
	unit, err := regular(m.Paths.Unit)
	if err != nil {
		return err
	}
	if string(unit) != string(m.definition(r.Config)) {
		return errors.New("service registration changed outside Harbor; preserve it before replacing or removing it")
	}
	return nil
}

func (m *Manager) configuration() (config.Config, error) {
	r, err := m.readRecord()
	if err != nil {
		return config.Config{}, err
	}
	return config.Load(r.Config)
}

// FromSource reads only installation ownership. Package maintenance must not
// require an unrelated custom installation's configuration or endpoint.
func (m *Manager) FromSource(source string) (bool, error) {
	r, err := m.readRecord()
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return r.Source == source, nil
}

func (m *Manager) quiescent(ctx context.Context, cfg config.Config) (func(), error) {
	// The controller retains this lock until utilities and owned ships have exited.
	// Do not replace an executable or remove its registration before that boundary.
	if _, err := os.Stat(filepath.Dir(cfg.DataDir)); os.IsNotExist(err) {
		return func() {}, nil
	}
	for {
		release, err := pier.Guard(cfg.DataDir)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Manager) Install(ctx context.Context, opts Options) error {
	if opts.Headless && m.Platform != "linux" {
		return errors.New("startup without a login is a Linux installation option")
	}
	release, err := m.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	r, err := m.readRecord()
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if opts.IfSource != "" && (!exists || r.Source != opts.IfSource) {
		return nil
	}
	if exists {
		if err = m.owned(r); err != nil {
			return err
		}
		if opts.Config != "" && filepath.Clean(opts.Config) != r.Config {
			return errors.New("this installation already uses a different configuration")
		}
	} else {
		for _, path := range []string{m.Paths.Unit, m.Paths.Binary} {
			if _, err = os.Lstat(path); err == nil {
				return fmt.Errorf("an existing service or runner needs inspection before installation: %s", path)
			}
			if !os.IsNotExist(err) {
				return err
			}
		}
		if err = m.registrationAvailable(ctx); err != nil && !(opts.NoStart && errors.Is(err, errNoSession)) {
			return err
		}
	}
	payload, err := regular(m.Source)
	if err != nil {
		return err
	}
	info, err := os.Stat(m.Source)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0111 == 0 {
		return errors.New("the downloaded Harbor runner is not executable")
	}
	changed := exists && digest(payload) != r.SHA256
	if changed && !opts.Upgrade {
		return errors.New("a newer runner is available; use harbor install --upgrade to update it gracefully")
	}
	if !exists {
		path := m.Paths.Config
		if opts.Config != "" {
			path = opts.Config
		}
		if !filepath.IsAbs(path) {
			return errors.New("configuration path must be absolute")
		}
		if _, err = os.Lstat(path); os.IsNotExist(err) && opts.Config == "" {
			defaults, encodeErr := json.Marshal(map[string]string{"dataDir": filepath.Dir(path)})
			if encodeErr != nil {
				return encodeErr
			}
			if err = atomicFile(path, append(defaults, '\n'), 0600); err != nil {
				return err
			}
		}
		if _, err = regular(path); err != nil {
			return err
		}
		cfg, loadErr := config.Load(path)
		if loadErr != nil {
			return loadErr
		}
		if _, err = localURL(cfg); err != nil {
			return err
		}
		r = record{Schema: 1, Source: m.Source, Config: filepath.Clean(path), Unit: m.Paths.Unit, SHA256: digest(payload), Enabled: !opts.NoStart}
		if m.Platform == "darwin" {
			if err = privateDir(m.Paths.Logs); err != nil {
				return err
			}
		}
		if err = atomicFile(m.Paths.Binary, payload, 0700); err != nil {
			return err
		}
		if err = atomicFile(m.Paths.Unit, m.definition(r.Config), 0600); err != nil {
			return err
		}
		if err = m.save(r); err != nil {
			return err
		}
		if m.Platform == "linux" {
			if _, err = m.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil && !errors.Is(err, errNoSession) {
				return err
			}
		}
	}
	if opts.Headless {
		if err = m.enableLinger(ctx); err != nil {
			return err
		}
		r.Headless = true
		if err = m.save(r); err != nil {
			return err
		}
	}
	if changed {
		return m.upgrade(ctx, r, payload)
	}
	if r.Enabled && !opts.NoStart {
		return m.start(ctx, r)
	}
	return nil
}

func (m *Manager) upgrade(ctx context.Context, r record, payload []byte) error {
	cfg, err := config.Load(r.Config)
	if err != nil {
		return err
	}
	old, err := regular(m.Paths.Binary)
	if err != nil {
		return err
	}
	previous := m.Paths.Binary + ".previous"
	if _, err = os.Lstat(previous); !os.IsNotExist(err) {
		return errors.New("an interrupted upgrade needs inspection; the previous runner has been preserved")
	}
	if err = atomicFile(previous, old, 0700); err != nil {
		return err
	}
	if err = m.stop(ctx, r); err != nil {
		os.Remove(previous)
		return err
	}
	release, err := m.quiescent(ctx, cfg)
	if err != nil {
		return err
	}
	if err = atomicFile(m.Paths.Binary, payload, 0700); err != nil {
		release()
		return err
	}
	updated := r
	updated.SHA256, updated.Source = digest(payload), m.Source
	err = m.save(updated)
	release()
	if err == nil && r.Enabled {
		err = m.start(ctx, updated)
		if errors.Is(err, errNoSession) {
			err = m.enableOfflineLinks()
		} // The enabled unit resumes at the next login.
	}
	if err == nil {
		return os.Remove(previous)
	}
	// A failed candidate must not strand the old installation. No forced kill is
	// used, including when a ship is still completing its graceful shutdown.
	rollback := m.stop(ctx, updated)
	if rollback != nil {
		return errors.Join(err, fmt.Errorf("rollback stop: %w; previous runner preserved", rollback))
	}
	release, rollback = m.quiescent(ctx, cfg)
	if rollback != nil {
		return errors.Join(err, rollback)
	}
	rollback = errors.Join(atomicFile(m.Paths.Binary, old, 0700), m.save(r))
	release()
	if rollback == nil && r.Enabled {
		rollback = m.start(ctx, r)
	}
	if rollback == nil {
		rollback = os.Remove(previous)
	}
	return errors.Join(err, rollback)
}

func (m *Manager) Start(ctx context.Context) error {
	release, err := m.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	r, err := m.readRecord()
	if err != nil {
		return err
	}
	if err = m.owned(r); err != nil {
		return err
	}
	r.Enabled = true
	if err = m.save(r); err != nil {
		return err
	}
	return m.start(ctx, r)
}

func (m *Manager) Stop(ctx context.Context) error {
	release, err := m.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	r, err := m.readRecord()
	if err != nil {
		return err
	}
	if err = m.owned(r); err != nil {
		return err
	}
	r.Enabled = false
	if err = m.save(r); err != nil {
		return err
	}
	if err = m.stop(ctx, r); err != nil {
		return err
	}
	cfg, err := config.Load(r.Config)
	if err != nil {
		return err
	}
	quiet, err := m.quiescent(ctx, cfg)
	if err == nil {
		quiet()
	}
	return err
}

func (m *Manager) Uninstall(ctx context.Context, ifSource string) error {
	release, err := m.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	r, err := m.readRecord()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if ifSource != "" && r.Source != ifSource {
		return nil
	}
	if err = m.owned(r); err != nil {
		return err
	}
	r.Enabled = false
	if err = m.save(r); err != nil {
		return err
	}
	if err = m.stop(ctx, r); err != nil {
		return err
	}
	cfg, err := config.Load(r.Config)
	if err != nil {
		return err
	}
	quiet, err := m.quiescent(ctx, cfg)
	if err != nil {
		return err
	}
	defer quiet()
	for _, path := range []string{m.Paths.Unit, m.Paths.Binary, m.Paths.Record} {
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	if m.Platform == "linux" {
		_, err = m.Run(ctx, "systemctl", "--user", "daemon-reload")
		if errors.Is(err, errNoSession) {
			err = nil
		}
	}
	return err
}
