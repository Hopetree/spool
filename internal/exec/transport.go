package exec

import (
	"context"
	"io/fs"
	"os"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/sshx"
)

// Runner is one host connection used by the execution engine.
type Runner interface {
	Upload(ctx context.Context, path string, data []byte, mode fs.FileMode) error
	RunStream(ctx context.Context, cmd string, onStdout, onStderr func([]byte)) (int, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	Close() error
}

// HostRef is the connection target snapshot.
type HostRef struct {
	ID              string
	Name            string
	Address         string
	Port            int
	CredentialID    string
	Fingerprint     string
	FingerprintType string
}

// RunnerFactory opens a Runner per host.
type RunnerFactory interface {
	Connect(ctx context.Context, h HostRef) (Runner, error)
}

// SSHFactory dials hosts with stored credentials and the configured key policy.
type SSHFactory struct {
	creds  *credential.Service
	policy string
}

// NewSSHFactory builds a production RunnerFactory.
func NewSSHFactory(creds *credential.Service, policy string) *SSHFactory {
	return &SSHFactory{creds: creds, policy: policy}
}

// Connect opens an SSH connection for a host.
func (f *SSHFactory) Connect(ctx context.Context, h HostRef) (Runner, error) {
	authType, username, secret, passphrase, err := f.creds.Open(ctx, h.CredentialID)
	if err != nil {
		return nil, err
	}
	client, err := sshx.Dial(ctx, sshx.Target{
		Address:             h.Address,
		Port:                h.Port,
		Username:            username,
		AuthType:            authType,
		Secret:              secret,
		Passphrase:          passphrase,
		ExpectedFingerprint: h.Fingerprint,
		Policy:              f.policy,
	})
	if err != nil {
		return nil, err
	}
	return &sshRunner{client: client}, nil
}

type sshRunner struct {
	client *sshx.Client
}

func (r *sshRunner) Upload(ctx context.Context, path string, data []byte, mode fs.FileMode) error {
	return r.client.Upload(ctx, path, data, os.FileMode(mode))
}

func (r *sshRunner) RunStream(ctx context.Context, cmd string, onStdout, onStderr func([]byte)) (int, error) {
	return r.client.RunStream(ctx, cmd, onStdout, onStderr)
}

func (r *sshRunner) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return r.client.ReadFile(ctx, path)
}

func (r *sshRunner) Close() error { return r.client.Close() }
