package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/sshx"
	"github.com/Hopetree/spool/internal/store"
)

// Errors returned by the transfer service.
var ErrNotFound = errors.New("transfer not found")

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

// Config tunes transfer limits.
type Config struct {
	TempDir          string
	MaxUploadBytes   int64
	MaxDownloadBytes int64
	Concurrency      int
}

// Transfer is the aggregate view of one file operation.
type Transfer struct {
	ID          string `json:"id"`
	Direction   string `json:"direction"`
	FileName    string `json:"file_name"`
	SizeBytes   int64  `json:"size_bytes"`
	TargetTotal int    `json:"target_total"`
	Status      string `json:"status"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   int64  `json:"created_at"`
	FinishedAt  *int64 `json:"finished_at"`
}

// Target is one host's progress within a transfer.
type Target struct {
	ID         string `json:"id"`
	TransferID string `json:"transfer_id"`
	HostID     string `json:"host_id"`
	HostName   string `json:"host_name"`
	RemotePath string `json:"remote_path"`
	Status     string `json:"status"`
	BytesDone  int64  `json:"bytes_done"`
	Error      string `json:"error"`
	StartedAt  *int64 `json:"started_at"`
	FinishedAt *int64 `json:"finished_at"`
}

type transferRow struct {
	ID          string `db:"id"`
	Direction   string `db:"direction"`
	FileName    string `db:"file_name"`
	SizeBytes   int64  `db:"size_bytes"`
	TargetTotal int    `db:"target_total"`
	Status      string `db:"status"`
	CreatedBy   string `db:"created_by"`
	CreatedAt   int64  `db:"created_at"`
	FinishedAt  *int64 `db:"finished_at"`
}

type targetRow struct {
	ID         string `db:"id"`
	TransferID string `db:"transfer_id"`
	HostID     string `db:"host_id"`
	HostName   string `db:"host_name_snapshot"`
	RemotePath string `db:"remote_path"`
	Status     string `db:"status"`
	BytesDone  int64  `db:"bytes_done"`
	Error      string `db:"error"`
	StartedAt  *int64 `db:"started_at"`
	FinishedAt *int64 `db:"finished_at"`
}

func (r transferRow) view() Transfer {
	return Transfer{
		ID: r.ID, Direction: r.Direction, FileName: r.FileName, SizeBytes: r.SizeBytes,
		TargetTotal: r.TargetTotal, Status: r.Status, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt,
	}
}

func (r targetRow) view() Target {
	return Target{
		ID: r.ID, TransferID: r.TransferID, HostID: r.HostID, HostName: r.HostName,
		RemotePath: r.RemotePath, Status: r.Status, BytesDone: r.BytesDone,
		Error: r.Error, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

type hostRef struct {
	ID              string `db:"id"`
	Name            string `db:"name"`
	Address         string `db:"address"`
	Port            int    `db:"port"`
	CredentialID    string `db:"credential_id"`
	Fingerprint     string `db:"fingerprint"`
	FingerprintType string `db:"fingerprint_type"`
	Enabled         bool   `db:"enabled"`
}

// UploadRequest carries one file distribution job.
type UploadRequest struct {
	FilePath      string
	FileName      string
	Size          int64
	HostIDs       []string
	RemoteDir     string
	RemoteName    string
	Mode          string
	Overwrite     string
	CreateParents bool
	ActorID       string
}

// Service runs SFTP uploads and downloads.
type Service struct {
	db     *sqlx.DB
	creds  *credential.Service
	policy string
	cfg    Config

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// NewService builds a transfer Service.
func NewService(db *sqlx.DB, creds *credential.Service, policy string, cfg Config) *Service {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.MaxUploadBytes <= 0 {
		cfg.MaxUploadBytes = 512 * 1024 * 1024
	}
	if cfg.MaxDownloadBytes <= 0 {
		cfg.MaxDownloadBytes = 200 * 1024 * 1024
	}
	if cfg.TempDir == "" {
		cfg.TempDir = os.TempDir()
	}
	return &Service{db: db, creds: creds, policy: policy, cfg: cfg, cancels: map[string]context.CancelFunc{}}
}

func (s *Service) nowMs() int64 { return time.Now().UnixMilli() }

func (s *Service) loadHost(ctx context.Context, id string) (hostRef, error) {
	var h hostRef
	err := s.db.GetContext(ctx, &h, `SELECT id, name, address, port, credential_id, fingerprint, fingerprint_type, enabled FROM hosts WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return hostRef{}, errors.New("主机不存在")
	}
	if err != nil {
		return hostRef{}, err
	}
	return h, nil
}

func (s *Service) dial(ctx context.Context, hostID string) (*sshx.Client, hostRef, error) {
	h, err := s.loadHost(ctx, hostID)
	if err != nil {
		return nil, h, err
	}
	if !h.Enabled {
		return nil, h, errors.New("主机已停用")
	}
	authType, username, secret, passphrase, err := s.creds.Open(ctx, h.CredentialID)
	if err != nil {
		return nil, h, err
	}
	client, err := sshx.Dial(ctx, sshx.Target{
		Address: h.Address, Port: h.Port, Username: username, AuthType: authType,
		Secret: secret, Passphrase: passphrase,
		ExpectedFingerprint: h.Fingerprint, Policy: s.policy,
	})
	if err != nil {
		return nil, h, err
	}
	return client, h, nil
}

// CreateUpload validates and starts a distribution job.
func (s *Service) CreateUpload(ctx context.Context, req UploadRequest) (*Transfer, error) {
	if req.Size <= 0 {
		return nil, invalid("文件为空")
	}
	if req.Size > s.cfg.MaxUploadBytes {
		return nil, invalid(fmt.Sprintf("文件超过上限（%d MB）", s.cfg.MaxUploadBytes/1024/1024))
	}
	if !strings.HasPrefix(req.RemoteDir, "/") {
		return nil, invalid("远端目录必须是绝对路径")
	}
	name := strings.TrimSpace(req.RemoteName)
	if name == "" {
		name = filepath.Base(req.FileName)
	}
	if strings.ContainsAny(name, "/\\") || name == "" || name == "." || name == ".." {
		return nil, invalid("远端文件名不合法")
	}
	if req.Mode == "" {
		req.Mode = "0644"
	}
	if _, err := strconv.ParseUint(req.Mode, 8, 32); err != nil {
		return nil, invalid("权限必须是八进制，例如 0644")
	}
	if req.Overwrite == "" {
		req.Overwrite = "overwrite"
	}
	if req.Overwrite != "overwrite" && req.Overwrite != "fail_if_exists" {
		return nil, invalid("覆盖策略必须是 overwrite 或 fail_if_exists")
	}
	hosts := make([]string, 0, len(req.HostIDs))
	seen := map[string]bool{}
	for _, id := range req.HostIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		hosts = append(hosts, id)
	}
	if len(hosts) == 0 {
		return nil, invalid("请选择至少一台主机")
	}
	q, args, err := sqlx.In(`SELECT count(*) FROM hosts WHERE id IN (?)`, hosts)
	if err != nil {
		return nil, err
	}
	var n int
	if err := s.db.GetContext(ctx, &n, s.db.Rebind(q), args...); err != nil {
		return nil, err
	}
	if n != len(hosts) {
		return nil, invalid("包含不存在的主机")
	}

	id := store.NewULID()
	now := s.nowMs()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO transfers (id, direction, file_name, size_bytes, target_total, status, created_by, created_at)
		VALUES (?, 'upload', ?, ?, ?, 'queued', ?, ?)`, id, name, req.Size, len(hosts), req.ActorID, now)
	if err != nil {
		return nil, fmt.Errorf("insert transfer: %w", err)
	}
	for _, hostID := range hosts {
		h, err := s.loadHost(ctx, hostID)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO transfer_targets (id, transfer_id, host_id, host_name_snapshot, remote_path, status)
			VALUES (?, ?, ?, ?, ?, 'queued')`, store.NewULID(), id, hostID, h.Name, path.Join(req.RemoteDir, name))
		if err != nil {
			return nil, fmt.Errorf("insert target: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transfer: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[id] = cancel
	s.mu.Unlock()
	go s.runUpload(runCtx, id, req, hosts)
	return s.getTransfer(id)
}

func (s *Service) getTransfer(id string) (*Transfer, error) {
	var r transferRow
	err := s.db.Get(&r, `SELECT * FROM transfers WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t := r.view()
	return &t, nil
}

// Get returns a transfer with its targets.
func (s *Service) Get(ctx context.Context, id string) (*Transfer, []Target, error) {
	var r transferRow
	err := s.db.GetContext(ctx, &r, `SELECT * FROM transfers WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get transfer: %w", err)
	}
	rows := []targetRow{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT * FROM transfer_targets WHERE transfer_id = ? ORDER BY host_name_snapshot`, id); err != nil {
		return nil, nil, fmt.Errorf("get targets: %w", err)
	}
	targets := make([]Target, 0, len(rows))
	for _, tr := range rows {
		targets = append(targets, tr.view())
	}
	t := r.view()
	return &t, targets, nil
}

// List returns a page of transfers.
func (s *Service) List(ctx context.Context, page, pageSize int) ([]Transfer, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := s.db.GetContext(ctx, &total, `SELECT count(*) FROM transfers`); err != nil {
		return nil, 0, err
	}
	rows := []transferRow{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT * FROM transfers ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		pageSize, (page-1)*pageSize); err != nil {
		return nil, 0, err
	}
	out := make([]Transfer, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.view())
	}
	return out, total, nil
}

// Cancel stops a running upload.
func (s *Service) Cancel(ctx context.Context, id string) error {
	if _, err := s.getTransfer(id); err != nil {
		return err
	}
	s.mu.Lock()
	cancel := s.cancels[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *Service) setTarget(id, status string, bytes int64, errMsg string, started, finished *int64) {
	_, _ = s.db.Exec(`UPDATE transfer_targets SET status = ?, bytes_done = ?, error = ?, started_at = COALESCE(?, started_at), finished_at = ? WHERE id = ?`,
		status, bytes, errMsg, started, finished, id)
}

func (s *Service) runUpload(ctx context.Context, id string, req UploadRequest, hostIDs []string) {
	defer func() {
		s.mu.Lock()
		delete(s.cancels, id)
		s.mu.Unlock()
		_ = os.Remove(req.FilePath)
	}()

	sem := make(chan struct{}, s.cfg.Concurrency)
	var wg sync.WaitGroup
	for _, hostID := range hostIDs {
		wg.Add(1)
		go func(hostID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			s.uploadOne(ctx, id, req, hostID)
		}(hostID)
	}
	wg.Wait()

	status := s.deriveUploadStatus(ctx, id)
	now := s.nowMs()
	_, _ = s.db.Exec(`UPDATE transfers SET status = ?, finished_at = ? WHERE id = ?`, status, now, id)
}

func (s *Service) deriveUploadStatus(ctx context.Context, id string) string {
	rows := []targetRow{}
	_ = s.db.Select(&rows, `SELECT * FROM transfer_targets WHERE transfer_id = ?`, id)
	if ctx.Err() != nil {
		return "cancelled"
	}
	var success, failed int
	for _, r := range rows {
		switch r.Status {
		case "success":
			success++
		case "failed":
			failed++
		case "cancelled":
			return "cancelled"
		}
	}
	switch {
	case failed == 0:
		return "success"
	case success == 0:
		return "failed"
	default:
		return "partial"
	}
}

func (s *Service) uploadOne(ctx context.Context, transferID string, req UploadRequest, hostID string) {
	var target targetRow
	if err := s.db.Get(&target, `SELECT * FROM transfer_targets WHERE transfer_id = ? AND host_id = ?`, transferID, hostID); err != nil {
		return
	}
	if ctx.Err() != nil {
		now := s.nowMs()
		s.setTarget(target.ID, "cancelled", 0, "已取消", nil, &now)
		return
	}
	start := s.nowMs()
	s.setTarget(target.ID, "running", 0, "", &start, nil)

	client, _, err := s.dial(ctx, hostID)
	if err != nil {
		now := s.nowMs()
		s.setTarget(target.ID, "failed", 0, err.Error(), nil, &now)
		return
	}
	defer client.Close()
	sftpClient, err := client.SFTP()
	if err != nil {
		now := s.nowMs()
		s.setTarget(target.ID, "failed", 0, "SFTP 不可用："+err.Error(), nil, &now)
		return
	}
	defer sftpClient.Close()

	remotePath := path.Join(req.RemoteDir, path.Base(target.RemotePath))
	if req.CreateParents {
		if err := sftpClient.MkdirAll(req.RemoteDir); err != nil {
			now := s.nowMs()
			s.setTarget(target.ID, "failed", 0, "创建目录失败："+err.Error(), nil, &now)
			return
		}
	}
	if req.Overwrite == "fail_if_exists" {
		if _, err := sftpClient.Stat(remotePath); err == nil {
			now := s.nowMs()
			s.setTarget(target.ID, "failed", 0, "远端文件已存在", nil, &now)
			return
		}
	}
	src, err := os.Open(req.FilePath)
	if err != nil {
		now := s.nowMs()
		s.setTarget(target.ID, "failed", 0, err.Error(), nil, &now)
		return
	}
	defer src.Close()
	dst, err := sftpClient.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		now := s.nowMs()
		s.setTarget(target.ID, "failed", 0, "打开远端文件失败："+err.Error(), nil, &now)
		return
	}

	mode64, _ := strconv.ParseUint(req.Mode, 8, 32)
	var written int64
	lastUpdate := time.Now()
	buf := make([]byte, 32*1024)
	var copyErr error
	for {
		if ctx.Err() != nil {
			copyErr = ctx.Err()
			break
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				copyErr = werr
				break
			}
			written += int64(n)
			if time.Since(lastUpdate) > 400*time.Millisecond {
				_, _ = s.db.Exec(`UPDATE transfer_targets SET bytes_done = ? WHERE id = ?`, written, target.ID)
				lastUpdate = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			copyErr = readErr
			break
		}
	}
	_ = dst.Close()
	if copyErr == nil {
		_ = sftpClient.Chmod(remotePath, os.FileMode(mode64))
	}
	now := s.nowMs()
	if copyErr != nil {
		status := "failed"
		msg := copyErr.Error()
		if ctx.Err() != nil {
			status = "cancelled"
			msg = "已取消"
		}
		s.setTarget(target.ID, status, written, msg, nil, &now)
		return
	}
	s.setTarget(target.ID, "success", written, "", nil, &now)
}

// Download streams a remote file to the caller.
type Download struct {
	Reader     io.Reader
	FileName   string
	Size       int64
	TransferID string
	once       sync.Once
	finish     func(error)
}

// Close reports the download outcome and releases the connection.
func (d *Download) Close(err error) {
	d.once.Do(func() { d.finish(err) })
}

// OpenDownload stats and opens a remote file for streaming.
func (s *Service) OpenDownload(ctx context.Context, hostID, remotePath, actorID string) (*Download, error) {
	if !strings.HasPrefix(remotePath, "/") {
		return nil, invalid("远端路径必须是绝对路径")
	}
	client, host, err := s.dial(ctx, hostID)
	if err != nil {
		return nil, err
	}
	sftpClient, err := client.SFTP()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("SFTP 不可用：%w", err)
	}
	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		sftpClient.Close()
		client.Close()
		return nil, fmt.Errorf("读取远端文件失败：%w", err)
	}
	if info.IsDir() {
		sftpClient.Close()
		client.Close()
		return nil, invalid("远端路径是目录")
	}
	if info.Size() > s.cfg.MaxDownloadBytes {
		sftpClient.Close()
		client.Close()
		return nil, invalid(fmt.Sprintf("文件超过下载上限（%d MB）", s.cfg.MaxDownloadBytes/1024/1024))
	}
	f, err := sftpClient.Open(remotePath)
	if err != nil {
		sftpClient.Close()
		client.Close()
		return nil, fmt.Errorf("打开远端文件失败：%w", err)
	}

	name := path.Base(remotePath)
	id := store.NewULID()
	now := s.nowMs()
	_, err = s.db.Exec(`INSERT INTO transfers (id, direction, file_name, size_bytes, target_total, status, created_by, created_at)
		VALUES (?, 'download', ?, ?, 1, 'running', ?, ?)`, id, name, info.Size(), actorID, now)
	if err != nil {
		f.Close()
		sftpClient.Close()
		client.Close()
		return nil, err
	}
	targetID := store.NewULID()
	_, err = s.db.Exec(`INSERT INTO transfer_targets (id, transfer_id, host_id, host_name_snapshot, remote_path, status, started_at)
		VALUES (?, ?, ?, ?, ?, 'running', ?)`, targetID, id, hostID, host.Name, remotePath, now)
	if err != nil {
		f.Close()
		sftpClient.Close()
		client.Close()
		return nil, err
	}

	finish := func(copyErr error) {
		finished := s.nowMs()
		status := "success"
		msg := ""
		if copyErr != nil {
			status = "failed"
			msg = copyErr.Error()
		}
		_, _ = s.db.Exec(`UPDATE transfer_targets SET status = ?, bytes_done = ?, error = ?, finished_at = ? WHERE id = ?`,
			status, info.Size(), msg, finished, targetID)
		_, _ = s.db.Exec(`UPDATE transfers SET status = ?, finished_at = ? WHERE id = ?`, status, finished, id)
		_ = f.Close()
		_ = sftpClient.Close()
		_ = client.Close()
	}
	return &Download{Reader: f, FileName: name, Size: info.Size(), TransferID: id, finish: finish}, nil
}
