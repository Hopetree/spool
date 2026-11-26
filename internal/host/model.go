package host

import "github.com/Hopetree/spool/internal/sshx"

// Host is the public view of a managed server.
type Host struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Address         string           `json:"address"`
	Port            int              `json:"port"`
	GroupID         *string          `json:"group_id"`
	GroupName       string           `json:"group_name"`
	CredentialID    string           `json:"credential_id,omitempty"`
	CredentialName  string           `json:"credential_name,omitempty"`
	Enabled         bool             `json:"enabled"`
	Fingerprint     string           `json:"fingerprint"`
	FingerprintType string           `json:"fingerprint_type"`
	Capability      sshx.ProbeResult `json:"capability"`
	LastTestAt      *int64           `json:"last_test_at"`
	LastTestOK      *bool            `json:"last_test_ok"`
	Note            string           `json:"note"`
	Tags            []string         `json:"tags"`
	CreatedAt       int64            `json:"created_at"`
	UpdatedAt       int64            `json:"updated_at"`
}

// Input carries create/update fields.
type Input struct {
	Name         string
	Address      string
	Port         int
	GroupID      string
	CredentialID string
	Enabled      bool
	Note         string
	Tags         []string
}

// Filter narrows host list queries.
type Filter struct {
	GroupID  string
	Tag      string
	Enabled  *bool
	Keyword  string
	Page     int
	PageSize int
}

// TestResult reports one connectivity test.
type TestResult struct {
	OK                  bool             `json:"ok"`
	LatencyMS           int64            `json:"latency_ms"`
	Fingerprint         string           `json:"fingerprint"`
	KeyType             string           `json:"key_type"`
	FingerprintRecorded bool             `json:"fingerprint_recorded"`
	Capability          sshx.ProbeResult `json:"capability"`
}

// Group is a flat host group.
type Group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	HostCount   int    `json:"host_count"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// GroupInput carries group create/update fields.
type GroupInput struct {
	Name        string
	Description string
}
