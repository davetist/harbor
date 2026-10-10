package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"harbor/internal/model"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

const codeRequest = "[0 %fyrd %base %khan-eval %noun %ted-eval '=/  m  (strand ,vase)  ;<  our=@p  bind:m  get-our  ;<  code=@p  bind:m  (scry @p /j/code/(scot %p our))  (pure:m !>((crip (slag 1 (scow %p code)))))']\n"

var codePattern = regexp.MustCompile(`^\[0 %avow 0 %noun (?:%([a-z]{6}(?:-[a-z]{6}){3})|'([a-z]{6}(?:-[a-z]{6}){3})')\]\s*$`)

func parseCode(decoded []byte) (string, error) {
	m := codePattern.FindSubmatch(decoded)
	if len(m) != 3 {
		return "", errors.New("ship did not return a successful access-code response")
	}
	if len(m[1]) > 0 {
		return string(m[1]), nil
	}
	return string(m[2]), nil
}

type legacyBoundedBuffer struct{ bytes.Buffer }

func (b *legacyBoundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxSensitive {
		return 0, errors.New("private query exceeds output limit")
	}
	return b.Buffer.Write(p)
}
func privateEval(ctx context.Context, binary string, args []string, input []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdin = bytes.NewReader(input)
	var out, diagnostic legacyBoundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	if cmd.Run() != nil {
		return nil, errors.New("private runtime query failed")
	}
	return out.Bytes(), nil
}
func readFrame(r io.Reader) ([]byte, error) {
	var header [5]byte
	if _, e := io.ReadFull(r, header[:]); e != nil {
		return nil, errors.New("private response header incomplete")
	}
	n := binary.LittleEndian.Uint32(header[1:])
	if header[0] != 0 || n == 0 || n > maxSensitive-5 {
		return nil, errors.New("invalid private response frame")
	}
	b := make([]byte, 5+int(n))
	copy(b, header[:])
	if _, e := io.ReadFull(r, b[5:]); e != nil {
		return nil, errors.New("private response incomplete")
	}
	return b, nil
}
func readAccessCodeViaConn(_ context.Context, ship model.Ship) (string, error) {
	// Finish the framed exchange even if the HTTP caller leaves. Vere's control
	// socket must not be abandoned while it is producing a response.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	encoded, e := privateEval(ctx, ship.RuntimePath, []string{"eval", "-jn"}, []byte(codeRequest))
	if e != nil {
		return "", e
	}
	if _, e = readFrame(bytes.NewReader(encoded)); e != nil {
		return "", e
	}
	socket := filepath.Join(ship.PierPath, ".urb", "conn.sock")
	conn, e := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if e != nil {
		return "", errors.New("ship control socket unavailable")
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	if _, e = io.Copy(conn, bytes.NewReader(encoded)); e != nil {
		return "", errors.New("private request failed")
	}
	response, e := readFrame(conn)
	if e != nil {
		return "", e
	}
	decoded, e := privateEval(ctx, ship.RuntimePath, []string{"eval", "-cn"}, response)
	if e != nil {
		return "", e
	}
	return parseCode(decoded)
}
