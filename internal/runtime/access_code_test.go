package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"harbor/internal/model"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestAccessCodeFramesAreBounded(t *testing.T) {
	for _, size := range []uint32{0, 1<<20 + 1} {
		var header [5]byte
		header[0] = 0
		binary.LittleEndian.PutUint32(header[1:], size)
		if _, e := readFrame(bytes.NewReader(header[:])); e == nil {
			t.Fatalf("accepted size %d", size)
		}
	}
}
func TestPartialSensitiveResponseFailsWithoutContents(t *testing.T) {
	var b bytes.Buffer
	b.WriteByte(0)
	binary.Write(&b, binary.LittleEndian, uint32(10))
	b.WriteString("secret")
	_, e := readFrame(&b)
	if e == nil || bytes.Contains([]byte(e.Error()), []byte("secret")) {
		t.Fatal("sensitive partial response escaped")
	}
}
func TestNewtFrameRoundtrip(t *testing.T) {
	data := []byte{0, 3, 0, 0, 0, 1, 2, 3}
	got, e := readFrame(bytes.NewReader(data))
	if e != nil || !bytes.Equal(got, data) {
		t.Fatalf("frame changed %v", e)
	}
	if _, e = readFrame(io.LimitReader(bytes.NewReader(data), 2)); e == nil {
		t.Fatal("truncated header accepted")
	}
}

// Opt-in only: never points at a user's live pier.
func TestDisposableAccessCodeLogin(t *testing.T) {
	binary, pier := os.Getenv("HARBOR_TEST_VERE"), os.Getenv("HARBOR_TEST_PIER")
	if binary == "" || pier == "" {
		t.Skip("set explicit disposable runtime and pier")
	}
	if !strings.Contains(pier, ".harbor-work/runtime-test/") {
		t.Fatal("only isolated disposable test piers are accepted")
	}
	code, e := ReadAccessCode(context.Background(), model.Ship{RuntimePath: binary, PierPath: pier})
	if e != nil {
		t.Fatal(e)
	}
	response, e := http.PostForm("http://127.0.0.1:18080/~/login", url.Values{"password": {code}})
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	if response.StatusCode != 204 && response.StatusCode != 200 && response.StatusCode != 303 {
		t.Fatalf("login rejected: HTTP %d", response.StatusCode)
	}
	if len(response.Cookies()) == 0 {
		t.Fatal("login returned no session cookie")
	}
	t.Log("private retrieval accepted by login endpoint; code omitted")
}

func TestDisposableResponseShape(t *testing.T) {
	if os.Getenv("HARBOR_RUN_LEGACY_CONN_PROBES") != "1" {
		t.Skip("explicit legacy conn probe required")
	}
	binary, pier := os.Getenv("HARBOR_TEST_VERE"), os.Getenv("HARBOR_TEST_PIER")
	if binary == "" || pier == "" {
		t.Skip("disposable runtime not supplied")
	}
	if !strings.Contains(pier, ".harbor-work/runtime-test/") {
		t.Fatal("invalid test pier")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	encoded, e := privateEval(ctx, binary, []string{"eval", "-jn"}, []byte(codeRequest))
	if e != nil {
		t.Fatal(e)
	}
	conn, e := net.DialTimeout("unix", filepath.Join(pier, ".urb", "conn.sock"), 5*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	conn.Write(encoded)
	response, e := readFrame(conn)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := privateEval(ctx, binary, []string{"eval", "-cn"}, response)
	if e != nil {
		t.Fatal(e)
	}
	// Show noun punctuation and tags, replace every possible code-like atom.
	safe := regexp.MustCompile(`[a-z][a-z-]{5,}`).ReplaceAllString(string(decoded), "REDACTED")
	t.Log("response structure:", safe)
}

func TestCodeDecoderAcceptsSuccessfulCordOnly(t *testing.T) {
	for _, v := range []string{"[0 %avow 0 %noun %sampel-sampel-sampel-sampel]\n", "[0 %avow 0 %noun 'sampel-sampel-sampel-sampel']\n"} {
		if _, e := parseCode([]byte(v)); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []string{"[0 %bail %sampel-sampel-sampel-sampel]", "[0 %avow 1 %noun %sampel-sampel-sampel-sampel]"} {
		if _, e := parseCode([]byte(v)); e == nil {
			t.Fatal("accepted failure as code")
		}
	}
}

func TestDisposableEarlySocketClose(t *testing.T) {
	if os.Getenv("HARBOR_RUN_LEGACY_CONN_PROBES") != "1" {
		t.Skip("explicit legacy conn chaos probe required")
	}
	binary, pier := os.Getenv("HARBOR_TEST_VERE"), os.Getenv("HARBOR_TEST_PIER")
	if binary == "" || pier == "" {
		t.Skip("explicit disposable pier not supplied")
	}
	if !strings.Contains(pier, ".harbor-work/runtime-test/") {
		t.Fatal("invalid test pier")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	encoded, e := privateEval(ctx, binary, []string{"eval", "-jn"}, []byte(codeRequest))
	if e != nil {
		t.Fatal(e)
	}
	conn, e := net.DialTimeout("unix", filepath.Join(pier, ".urb", "conn.sock"), 3*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = conn.Write(encoded); e != nil {
		t.Fatal(e)
	}
	conn.Close()
	time.Sleep(time.Second)
	client := &http.Client{Timeout: 3 * time.Second}
	r, e := client.Get("http://127.0.0.1:18080/~/login")
	if e != nil {
		t.Fatal("disposable ship did not survive early socket close")
	}
	r.Body.Close()
	if r.StatusCode >= 500 {
		t.Fatal("ship unhealthy after early close")
	}
}
