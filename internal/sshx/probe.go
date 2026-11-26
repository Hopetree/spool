package sshx

import (
	"context"
	"strings"
	"time"
)

// ProbeResult is the cached capability snapshot of a host.
type ProbeResult struct {
	User          string `json:"user"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	Arch          string `json:"arch"`
	Distro        string `json:"distro"`
	BashPath      string `json:"bash_path"`
	BashVersion   string `json:"bash_version"`
	PythonPath    string `json:"python_path"`
	PythonVersion string `json:"python_version"`
	SFTP          bool   `json:"sftp"`
	Sudo          bool   `json:"sudo"`
	Setsid        bool   `json:"setsid"`
	Timeout       bool   `json:"timeout"`
	LatencyMS     int64  `json:"latency_ms"`
	Fingerprint   string `json:"fingerprint"`
	KeyType       string `json:"key_type"`
	ProbedAt      int64  `json:"probed_at"`
}

const probeScript = `echo "SPOOL_USER=$(id -un 2>/dev/null)"
echo "SPOOL_OS=$(uname -s 2>/dev/null)"
echo "SPOOL_KERNEL=$(uname -r 2>/dev/null)"
echo "SPOOL_ARCH=$(uname -m 2>/dev/null)"
if [ -r /etc/os-release ]; then . /etc/os-release 2>/dev/null; echo "SPOOL_DISTRO=${PRETTY_NAME:-$ID}"; fi
echo "SPOOL_BASH_PATH=$(command -v bash 2>/dev/null)"
echo "SPOOL_BASH_VERSION=$(bash --version 2>/dev/null | head -n1)"
echo "SPOOL_PYTHON_PATH=$(command -v python3 2>/dev/null)"
echo "SPOOL_PYTHON_VERSION=$(python3 --version 2>/dev/null)"
command -v setsid >/dev/null 2>&1 && echo "SPOOL_SETSID=1" || echo "SPOOL_SETSID=0"
command -v timeout >/dev/null 2>&1 && echo "SPOOL_TIMEOUT=1" || echo "SPOOL_TIMEOUT=0"
sudo -n true >/dev/null 2>&1 && echo "SPOOL_SUDO=1" || echo "SPOOL_SUDO=0"`

// Prober connects to a host and collects its capability snapshot.
type Prober struct{}

// NewProber builds a Prober.
func NewProber() *Prober { return &Prober{} }

// Probe dials the target, runs the probe script and verifies SFTP.
func (p *Prober) Probe(ctx context.Context, t Target) (ProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	start := time.Now()
	c, err := Dial(ctx, t)
	if err != nil {
		return ProbeResult{}, err
	}
	defer c.Close()

	res := ProbeResult{
		Fingerprint: c.fingerprint,
		KeyType:     c.keyType,
		LatencyMS:   time.Since(start).Milliseconds(),
		ProbedAt:    time.Now().UnixMilli(),
	}
	stdout, _, _, err := c.Run(ctx, probeScript)
	if err != nil {
		return res, err
	}
	parseProbe(stdout, &res)

	if sc, err := c.SFTP(); err == nil {
		res.SFTP = true
		_ = sc.Close()
	}
	return res, nil
}

func parseProbe(out []byte, res *ProbeResult) {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "SPOOL_USER":
			res.User = val
		case "SPOOL_OS":
			res.OS = val
		case "SPOOL_KERNEL":
			res.Kernel = val
		case "SPOOL_ARCH":
			res.Arch = val
		case "SPOOL_DISTRO":
			res.Distro = val
		case "SPOOL_BASH_PATH":
			res.BashPath = val
		case "SPOOL_BASH_VERSION":
			res.BashVersion = val
		case "SPOOL_PYTHON_PATH":
			res.PythonPath = val
		case "SPOOL_PYTHON_VERSION":
			res.PythonVersion = val
		case "SPOOL_SETSID":
			res.Setsid = val == "1"
		case "SPOOL_TIMEOUT":
			res.Timeout = val == "1"
		case "SPOOL_SUDO":
			res.Sudo = val == "1"
		}
	}
}
