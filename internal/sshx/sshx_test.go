package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	xssh "golang.org/x/crypto/ssh"
)

const probeFixture = `SPOOL_USER=root
SPOOL_OS=Linux
SPOOL_KERNEL=6.1.0-test
SPOOL_ARCH=x86_64
SPOOL_DISTRO=Ubuntu 22.04 LTS
SPOOL_BASH_PATH=/bin/bash
SPOOL_BASH_VERSION=GNU bash, version 5.1.16(1)-release
SPOOL_PYTHON_PATH=/usr/bin/python3
SPOOL_PYTHON_VERSION=Python 3.10.12
SPOOL_SETSID=1
SPOOL_TIMEOUT=1
SPOOL_SUDO=0
`

type fakeHost struct {
	addr    string
	fp      string
	user    string
	pass    string
	privKey ed25519.PrivateKey
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := xssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	srv := &gssh.Server{
		SubsystemHandlers: map[string]gssh.SubsystemHandler{
			"sftp": func(s gssh.Session) {
				ss, err := sftp.NewServer(s)
				if err != nil {
					_ = s.Exit(1)
					return
				}
				defer ss.Close()
				_ = ss.Serve()
			},
		},
		Handler: func(s gssh.Session) {
			cmd := s.RawCommand()
			switch {
			case strings.Contains(cmd, "SPOOL_USER"):
				_, _ = io.WriteString(s, probeFixture)
			case strings.Contains(cmd, "stderr-test"):
				_, _ = io.WriteString(s, "out\n")
				_, _ = io.WriteString(s.Stderr(), "boom\n")
			case strings.Contains(cmd, "exit3"):
				_ = s.Exit(3)
			default:
				_, _ = io.WriteString(s, "hi\n")
			}
		},
		PasswordHandler: func(ctx gssh.Context, password string) bool {
			return ctx.User() == "root" && password == "pw"
		},
		PublicKeyHandler: func(ctx gssh.Context, key gssh.PublicKey) bool {
			return ctx.User() == "root" && gssh.KeysEqual(key, signer.PublicKey())
		},
	}
	srv.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	time.Sleep(20 * time.Millisecond)
	return &fakeHost{
		addr:    ln.Addr().String(),
		fp:      xssh.FingerprintSHA256(signer.PublicKey()),
		user:    "root",
		pass:    "pw",
		privKey: priv,
	}
}

func (h *fakeHost) target(policy, expected string) Target {
	host, portStr, _ := net.SplitHostPort(h.addr)
	port, _ := strconv.Atoi(portStr)
	return Target{
		Address: host, Port: port, Username: h.user, AuthType: "password",
		Secret: h.pass, Policy: policy, ExpectedFingerprint: expected,
	}
}

func TestDialAndRun(t *testing.T) {
	h := newFakeHost(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx, h.target("tofu", h.fp))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	stdout, stderr, code, err := c.Run(ctx, "echo hi")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(stdout) != "hi\n" || string(stderr) != "" || code != 0 {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestRunStderrAndExitCode(t *testing.T) {
	h := newFakeHost(t)
	ctx := context.Background()
	c, err := Dial(ctx, h.target("tofu", h.fp))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	stdout, stderr, code, err := c.Run(ctx, "stderr-test")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(stdout) != "out\n" || string(stderr) != "boom\n" || code != 0 {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	_, _, code, err = c.Run(ctx, "exit3")
	if err != nil {
		t.Fatalf("exit3 run error: %v", err)
	}
	if code != 3 {
		t.Fatalf("code = %d, want 3", code)
	}
}

func TestWrongPassword(t *testing.T) {
	h := newFakeHost(t)
	target := h.target("tofu", h.fp)
	target.Secret = "wrong"
	if _, err := Dial(context.Background(), target); err == nil {
		t.Fatal("expected auth failure")
	}
}

func TestTOFUAcceptsUnknownAndReportsFingerprint(t *testing.T) {
	h := newFakeHost(t)
	c, err := Dial(context.Background(), h.target("tofu", ""))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	fp, kt := c.Fingerprint()
	if fp != h.fp || kt != "ssh-ed25519" {
		t.Fatalf("fingerprint = %q %q", fp, kt)
	}
}

func TestStrictRejectsUnknownFingerprint(t *testing.T) {
	h := newFakeHost(t)
	_, err := Dial(context.Background(), h.target("strict", ""))
	var keyErr *HostKeyError
	if !errors.As(err, &keyErr) {
		t.Fatalf("err = %v, want HostKeyError", err)
	}
	if keyErr.Changed || keyErr.Actual != h.fp || keyErr.KeyType != "ssh-ed25519" {
		t.Fatalf("keyErr = %+v", keyErr)
	}
}

func TestFingerprintChanged(t *testing.T) {
	h := newFakeHost(t)
	_, err := Dial(context.Background(), h.target("tofu", "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	var keyErr *HostKeyError
	if !errors.As(err, &keyErr) {
		t.Fatalf("err = %v, want HostKeyError", err)
	}
	if !keyErr.Changed || keyErr.Actual != h.fp {
		t.Fatalf("keyErr = %+v", keyErr)
	}
}

func TestPrivateKeyAuth(t *testing.T) {
	h := newFakeHost(t)
	der, err := x509.MarshalPKCS8PrivateKey(h.privKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	target := h.target("tofu", h.fp)
	target.AuthType = "private_key"
	target.Secret = string(keyPEM)
	c, err := Dial(context.Background(), target)
	if err != nil {
		t.Fatalf("dial with key: %v", err)
	}
	defer c.Close()
	if _, _, code, err := c.Run(context.Background(), "echo hi"); err != nil || code != 0 {
		t.Fatalf("run: code=%d err=%v", code, err)
	}
}

func TestProbe(t *testing.T) {
	h := newFakeHost(t)
	p := NewProber()
	res, err := p.Probe(context.Background(), h.target("tofu", h.fp))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if res.User != "root" || res.OS != "Linux" || res.Kernel != "6.1.0-test" ||
		res.Arch != "x86_64" || res.Distro != "Ubuntu 22.04 LTS" ||
		res.BashPath != "/bin/bash" || res.PythonPath != "/usr/bin/python3" ||
		res.PythonVersion != "Python 3.10.12" {
		t.Fatalf("probe result = %+v", res)
	}
	if !res.SFTP || res.Sudo || !res.Setsid || !res.Timeout {
		t.Fatalf("bool flags = %+v", res)
	}
	if res.Fingerprint != h.fp || res.KeyType != "ssh-ed25519" {
		t.Fatalf("fingerprint = %+v", res)
	}
}

func TestSFTPSubsystem(t *testing.T) {
	h := newFakeHost(t)
	c, err := Dial(context.Background(), h.target("tofu", h.fp))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	sc, err := c.SFTP()
	if err != nil {
		t.Fatalf("sftp client: %v", err)
	}
	defer sc.Close()
	if _, err := sc.Stat("/"); err != nil {
		t.Fatalf("stat /: %v", err)
	}
}

func TestRunStream(t *testing.T) {
	h := newFakeHost(t)
	c, err := Dial(context.Background(), h.target("tofu", h.fp))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var mu sync.Mutex
	var out, errOut bytes.Buffer
	code, err := c.RunStream(context.Background(), "echo hi", func(b []byte) {
		mu.Lock()
		out.Write(b)
		mu.Unlock()
	}, func(b []byte) {
		mu.Lock()
		errOut.Write(b)
		mu.Unlock()
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if out.String() != "hi\n" || errOut.String() != "" {
		t.Fatalf("out=%q err=%q", out.String(), errOut.String())
	}
}

func TestUploadAndReadFile(t *testing.T) {
	h := newFakeHost(t)
	c, err := Dial(context.Background(), h.target("tofu", h.fp))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := filepath.Join(t.TempDir(), "script.sh")
	if err := c.Upload(context.Background(), p, []byte("echo uploaded"), 0o600); err != nil {
		t.Fatalf("upload: %v", err)
	}
	b, err := c.ReadFile(context.Background(), p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "echo uploaded" {
		t.Fatalf("content = %q", b)
	}
}
