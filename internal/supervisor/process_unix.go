package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func StartIdentity(pid int) (string, error) {
	if pid <= 1 {
		return "", errors.New("invalid process ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "/bin/ps", "-p", fmt.Sprint(pid), "-o", "lstart=", "-o", "comm=").Output()
	if e != nil || len(strings.TrimSpace(string(b))) == 0 {
		return "", errors.New("process is not observable")
	}
	return strings.TrimSpace(string(b)), nil
}
