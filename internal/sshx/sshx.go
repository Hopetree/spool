package sshx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/sftp"
	xssh "golang.org/x/crypto/ssh"
)

// Target describes one SSH connection attempt.
type Target struct {
	Address             string
	Port                int
	Username            string
	AuthType            string // password | private_key
	Secret              string
	Passphrase          string
	ExpectedFingerprint string
	Policy              string // tofu | strict
}

// HostKeyError reports an unknown or changed host key.
type HostKeyError struct {
	Expected string
	Actual   string
	KeyType  string
	Changed  bool
}

func (e *HostKeyError) Error() string {
	if e.Changed {
		return fmt.Sprintf("host key changed: expected %s, got %s", e.Expected, e.Actual)
	}
	return fmt.Sprintf("unknown host key %s (%s)", e.Actual, e.KeyType)
}

// Client wraps an established SSH connection.
type Client struct {
	conn        *xssh.Client
	fingerprint string
	keyType     string
}

// Fingerprint returns the negotiated host key fingerprint and type.
func (c *Client) Fingerprint() (string, string) { return c.fingerprint, c.keyType }

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

// SFTP opens an SFTP subsystem on the connection.
func (c *Client) SFTP() (*sftp.Client, error) { return sftp.NewClient(c.conn) }

func authMethods(t Target) ([]xssh.AuthMethod, error) {
	switch t.AuthType {
	case "password":
		return []xssh.AuthMethod{xssh.Password(t.Secret)}, nil
	case "private_key":
		var (
			signer xssh.Signer
			err    error
		)
		if t.Passphrase != "" {
			signer, err = xssh.ParsePrivateKeyWithPassphrase([]byte(t.Secret), []byte(t.Passphrase))
		} else {
			signer, err = xssh.ParsePrivateKey([]byte(t.Secret))
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return []xssh.AuthMethod{xssh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("unsupported auth type %q", t.AuthType)
	}
}

// Dial establishes an SSH connection and enforces the host key policy.
func Dial(ctx context.Context, t Target) (*Client, error) {
	if t.Port == 0 {
		t.Port = 22
	}
	if t.Policy == "" {
		t.Policy = "tofu"
	}
	auth, err := authMethods(t)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var actualFP, actualKT string
	var keyErr *HostKeyError
	callback := func(hostname string, remote net.Addr, key xssh.PublicKey) error {
		actualFP = xssh.FingerprintSHA256(key)
		actualKT = key.Type()
		if t.ExpectedFingerprint == "" {
			if t.Policy == "strict" {
				keyErr = &HostKeyError{Actual: actualFP, KeyType: actualKT}
				return keyErr
			}
			return nil
		}
		if t.ExpectedFingerprint != actualFP {
			keyErr = &HostKeyError{Expected: t.ExpectedFingerprint, Actual: actualFP, KeyType: actualKT, Changed: true}
			return keyErr
		}
		return nil
	}

	conf := &xssh.ClientConfig{
		User:            t.Username,
		Auth:            auth,
		HostKeyCallback: callback,
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort(t.Address, strconv.Itoa(t.Port))
	dialer := net.Dialer{Timeout: 10 * time.Second}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	conn, chans, reqs, err := xssh.NewClientConn(raw, addr, conf)
	if err != nil {
		_ = raw.Close()
		if keyErr != nil {
			return nil, keyErr
		}
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	_ = raw.SetDeadline(time.Time{})
	return &Client{
		conn:        xssh.NewClient(conn, chans, reqs),
		fingerprint: actualFP,
		keyType:     actualKT,
	}, nil
}

// Run executes a command and captures stdout/stderr and the exit code. A
// non-zero exit status is returned via exitCode, not err.
func (c *Client) Run(ctx context.Context, command string) (stdout, stderr []byte, exitCode int, err error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, -1, fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()

	var out, errBuf bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &errBuf

	done := make(chan error, 1)
	go func() { done <- sess.Run(command) }()

	select {
	case runErr := <-done:
		if runErr == nil {
			return out.Bytes(), errBuf.Bytes(), 0, nil
		}
		var exitErr *xssh.ExitError
		if errors.As(runErr, &exitErr) {
			return out.Bytes(), errBuf.Bytes(), exitErr.ExitStatus(), nil
		}
		return out.Bytes(), errBuf.Bytes(), -1, fmt.Errorf("run: %w", runErr)
	case <-ctx.Done():
		_ = sess.Signal(xssh.SIGTERM)
		_ = sess.Close()
		<-done
		return out.Bytes(), errBuf.Bytes(), -1, ctx.Err()
	}
}

// Upload writes data to a remote path over SFTP.
func (c *Client) Upload(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	sc, err := c.SFTP()
	if err != nil {
		return fmt.Errorf("sftp: %w", err)
	}
	defer sc.Close()
	f, err := sc.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("open remote file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write remote file: %w", err)
	}
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("chmod remote file: %w", err)
	}
	return nil
}

// ReadFile reads a remote file over SFTP.
func (c *Client) ReadFile(ctx context.Context, path string) ([]byte, error) {
	sc, err := c.SFTP()
	if err != nil {
		return nil, fmt.Errorf("sftp: %w", err)
	}
	defer sc.Close()
	f, err := sc.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open remote file: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read remote file: %w", err)
	}
	return b, nil
}

func pump(r io.Reader, fn func([]byte)) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 && fn != nil {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			fn(chunk)
		}
		if err != nil {
			return
		}
	}
}

// RunStream executes a command and streams stdout/stderr chunks as they arrive.
// A non-zero exit status is returned via exitCode, not err.
func (c *Client) RunStream(ctx context.Context, command string, onStdout, onStderr func([]byte)) (int, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return -1, fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return -1, err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return -1, err
	}
	if err := sess.Start(command); err != nil {
		return -1, fmt.Errorf("start: %w", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); pump(stdout, onStdout) }()
	go func() { defer wg.Done(); pump(stderr, onStderr) }()
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()

	select {
	case runErr := <-done:
		wg.Wait()
		if runErr == nil {
			return 0, nil
		}
		var exitErr *xssh.ExitError
		if errors.As(runErr, &exitErr) {
			return exitErr.ExitStatus(), nil
		}
		return -1, fmt.Errorf("run: %w", runErr)
	case <-ctx.Done():
		_ = sess.Signal(xssh.SIGTERM)
		_ = sess.Close()
		<-done
		wg.Wait()
		return -1, ctx.Err()
	}
}
