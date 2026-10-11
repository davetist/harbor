package installation

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func systemdQuote(s string) string {
	s = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t", "%", "%%", "$", "$$").Replace(s)
	return "\"" + s + "\""
}

func xmlString(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }

func (m *Manager) definition(configPath string) []byte {
	if m.Platform == "linux" {
		return []byte(fmt.Sprintf("# Managed by Harbor; ship data is retained on removal.\n[Unit]\nDescription=Harbor Urbit ship runner\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=%s serve --config %s\nRestart=on-failure\nRestartSec=5\nKillSignal=SIGTERM\nKillMode=process\nTimeoutStopSec=infinity\nUMask=0077\n\n[Install]\nWantedBy=default.target\n", systemdQuote(m.Paths.Binary), systemdQuote(configPath)))
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>serve</string><string>--config</string><string>%s</string></array>
<key>StandardOutPath</key><string>%s/service.log</string>
<key>StandardErrorPath</key><string>%s/error.log</string>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
<key>AbandonProcessGroup</key><true/>
<key>ExitTimeOut</key><integer>0</integer>
<key>Umask</key><integer>63</integer>
</dict></plist>
`, xmlString(m.Paths.Label), xmlString(m.Paths.Binary), xmlString(configPath), xmlString(m.Paths.Logs), xmlString(m.Paths.Logs)))
}

func (m *Manager) registrationAvailable(ctx context.Context) error {
	if m.Platform == "linux" {
		out, err := m.Run(ctx, "systemctl", "--user", "show", m.Paths.Label+".service", "--property=FragmentPath", "--value")
		if err != nil {
			if errors.Is(err, errNoSession) {
				return err
			}
			return errors.New("the user service manager is unavailable; install from a normal login or SSH session on a systemd host")
		}
		if path := strings.TrimSpace(string(out)); path != "" {
			return errors.New("another Harbor service is already registered; preserve it before installing")
		}
	} else if _, err := m.Run(ctx, "launchctl", "print", "gui/"+m.UID+"/"+m.Paths.Label); err == nil {
		return errors.New("another Harbor service is already registered; preserve it before installing")
	}
	return nil
}

func (m *Manager) servicePID(ctx context.Context) int {
	if m.Platform == "linux" {
		out, err := m.Run(ctx, "systemctl", "--user", "show", m.Paths.Label+".service", "--property=MainPID", "--value")
		if err != nil {
			return 0
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(string(out)))
		return pid
	}
	out, err := m.Run(ctx, "launchctl", "print", "gui/"+m.UID+"/"+m.Paths.Label)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "pid" && fields[1] == "=" {
			pid, _ := strconv.Atoi(fields[2])
			return pid
		}
	}
	return 0
}

func (m *Manager) start(ctx context.Context, r record) error {
	if m.Platform == "linux" {
		if _, err := m.Run(ctx, "systemctl", "--user", "enable", "--now", m.Paths.Unit); err != nil {
			return err
		}
	} else {
		domain := "gui/" + m.UID
		if _, err := m.Run(ctx, "launchctl", "enable", domain+"/"+m.Paths.Label); err != nil {
			return err
		}
		if _, err := m.Run(ctx, "launchctl", "print", domain+"/"+m.Paths.Label); err != nil {
			if _, err = m.Run(ctx, "launchctl", "bootstrap", domain, m.Paths.Unit); err != nil {
				return err
			}
		} else if _, err = m.Run(ctx, "launchctl", "kickstart", domain+"/"+m.Paths.Label); err != nil {
			return err
		}
	}
	return m.waitReady(ctx, r.Config)
}

func (m *Manager) stop(ctx context.Context, r record) error {
	if m.Platform == "linux" {
		_, err := m.Run(ctx, "systemctl", "--user", "disable", "--now", m.Paths.Label+".service")
		if errors.Is(err, errNoSession) {
			return m.removeOfflineLinks()
		}
		return err
	}
	domain := "gui/" + m.UID
	if _, err := m.Run(ctx, "launchctl", "disable", domain+"/"+m.Paths.Label); err != nil {
		return err
	}
	if _, err := m.Run(ctx, "launchctl", "print", domain+"/"+m.Paths.Label); err == nil {
		_, err = m.Run(ctx, "launchctl", "bootout", domain, m.Paths.Unit)
		return err
	}
	return nil
}

func (m *Manager) removeOfflineLinks() error {
	// A logged-out desktop user can have an enabled unit but no user bus.
	// Remove only links to this installation; the controller lock is still
	// required before an executable or registration can be removed/replaced.
	dirs := []string{filepath.Dir(m.Paths.Unit), filepath.Join(m.Paths.Home, ".config", "systemd", "user")}
	seen := map[string]bool{}
	for _, dir := range dirs {
		for _, path := range []string{filepath.Join(dir, "default.target.wants", m.Paths.Label+".service"), filepath.Join(dir, m.Paths.Label+".service")} {
			if path == m.Paths.Unit || seen[path] {
				continue
			}
			seen[path] = true
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				return errors.New("an unexpected service link needs inspection")
			}
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			if filepath.Clean(target) != m.Paths.Unit {
				return errors.New("an unrelated service link needs inspection")
			}
			if err = os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) enableOfflineLinks() error {
	dirs := []string{filepath.Dir(m.Paths.Unit), filepath.Join(m.Paths.Home, ".config", "systemd", "user")}
	for _, dir := range dirs {
		for _, path := range []string{filepath.Join(dir, m.Paths.Label+".service"), filepath.Join(dir, "default.target.wants", m.Paths.Label+".service")} {
			if path == m.Paths.Unit {
				continue
			}
			if _, err := os.Lstat(path); err == nil {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(path), target)
				}
				if filepath.Clean(target) != m.Paths.Unit {
					return errors.New("an unrelated service link needs inspection")
				}
				continue
			} else if !os.IsNotExist(err) {
				return err
			}
			if err := privateDir(filepath.Dir(path)); err != nil {
				return err
			}
			if err := os.Symlink(m.Paths.Unit, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) enableLinger(ctx context.Context) error {
	if out, err := m.Run(ctx, "loginctl", "show-user", m.UID, "--property=Linger", "--value"); err == nil && strings.TrimSpace(string(out)) == "yes" {
		return nil
	}
	if _, err := m.Run(ctx, "loginctl", "enable-linger", m.Username); err == nil {
		return nil
	}
	if _, err := m.Run(ctx, "sudo", "loginctl", "enable-linger", m.Username); err != nil {
		return errors.New("startup without login needs administrator approval for user lingering; rerun harbor install --headless in an interactive terminal")
	}
	out, err := m.Run(ctx, "loginctl", "show-user", m.UID, "--property=Linger", "--value")
	if err != nil || strings.TrimSpace(string(out)) != "yes" {
		return errors.New("could not verify startup without a login")
	}
	return nil
}
