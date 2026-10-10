package runtime

import (
	"context"
	"errors"
	"harbor/internal/model"
	"os/exec"
	"strings"
)

type Capabilities struct {
	Operations    []model.Operation
	LoomExponents []int
	Version       string
}

func ProbeCapabilities(ctx context.Context, binary string) (Capabilities, error) {
	p, e := NativePlatform()
	if e != nil {
		return Capabilities{}, e
	}
	if e = ValidateBinary(binary, p); e != nil {
		return Capabilities{}, e
	}
	cmd := exec.CommandContext(ctx, binary, "-R")
	var out boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if cmd.Run() != nil || !strings.Contains(out.String(), "4.6") {
		return Capabilities{}, errors.New("runtime version is not supported")
	}
	return Capabilities{Operations: []model.Operation{"start", "pack", "meld", "roll", "chop"}, LoomExponents: []int{30, 31, 32}, Version: StableVersion}, nil
}
