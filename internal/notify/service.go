package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/store"
)

// Errors returned by the notify service.
var ErrNotFound = errors.New("channel not found")

// ValidationError marks invalid input.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(msg string) error { return &ValidationError{Message: msg} }

// Channel is the public view of a notification channel.
type Channel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	HasSecret bool   `json:"has_secret"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Input carries channel create/update fields. Empty Secret means unchanged.
type Input struct {
	Name    string
	Type    string
	URL     string
	Secret  string
	Enabled bool
}

type row struct {
	ID         string `db:"id"`
	Name       string `db:"name"`
	Type       string `db:"type"`
	ConfigJSON string `db:"config_json"`
	SecretEnc  []byte `db:"secret_enc"`
	Enabled    bool   `db:"enabled"`
	CreatedAt  int64  `db:"created_at"`
	UpdatedAt  int64  `db:"updated_at"`
}

type channelConfig struct {
	URL string `json:"url"`
}

func (r row) toChannel() Channel {
	cfg := channelConfig{}
	_ = json.Unmarshal([]byte(r.ConfigJSON), &cfg)
	return Channel{
		ID: r.ID, Name: r.Name, Type: r.Type, URL: cfg.URL,
		HasSecret: len(r.SecretEnc) > 0, Enabled: r.Enabled,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// ExecutionNotice is the subset of execution data used for messages.
type ExecutionNotice struct {
	ID            string
	Name          string
	Status        string
	FailureReason string
	ErrorSummary  string
	ScheduleID    string
	HostTotal     int
	HostSuccess   int
	HostFailed    int
	HostCancelled int
	StartedAt     *int64
	FinishedAt    *int64
}

// Service stores channels and sends notifications.
type Service struct {
	db      *sqlx.DB
	creds   *credential.Service
	baseURL string
	client  *http.Client
	now     func() time.Time
}

// NewService builds a notify Service.
func NewService(db *sqlx.DB, creds *credential.Service, baseURL string) *Service {
	return &Service{
		db: db, creds: creds, baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}
}

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

func validateInput(in Input) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 {
		return invalid("渠道名称不能为空且不能超过 64 个字符")
	}
	switch in.Type {
	case "webhook", "feishu", "dingtalk", "wecom":
	default:
		return invalid("渠道类型必须是 webhook/feishu/dingtalk/wecom")
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return invalid("请填写有效的 http(s) Webhook 地址")
	}
	return nil
}

// List returns all channels.
func (s *Service) List(ctx context.Context) ([]Channel, error) {
	rows := []row{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT id, name, type, config_json, secret_enc, enabled, created_at, updated_at FROM notify_channels ORDER BY name`); err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	out := make([]Channel, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toChannel())
	}
	return out, nil
}

func (s *Service) load(ctx context.Context, id string) (row, error) {
	var r row
	err := s.db.GetContext(ctx, &r, `SELECT id, name, type, config_json, secret_enc, enabled, created_at, updated_at FROM notify_channels WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, ErrNotFound
	}
	if err != nil {
		return row{}, fmt.Errorf("get channel: %w", err)
	}
	return r, nil
}

// Get returns one channel.
func (s *Service) Get(ctx context.Context, id string) (*Channel, error) {
	r, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	ch := r.toChannel()
	return &ch, nil
}

func (s *Service) saveSecret(id, secret string) ([]byte, error) {
	if secret == "" {
		return nil, nil
	}
	return s.creds.EncryptSensitive([]byte(secret), id)
}

// Create inserts a channel.
func (s *Service) Create(ctx context.Context, in Input) (*Channel, error) {
	if err := validateInput(in); err != nil {
		return nil, err
	}
	id := store.NewULID()
	secretEnc, err := s.saveSecret(id, in.Secret)
	if err != nil {
		return nil, err
	}
	cfg, _ := json.Marshal(channelConfig{URL: strings.TrimSpace(in.URL)})
	now := s.nowMs()
	_, err = s.db.ExecContext(ctx, `INSERT INTO notify_channels (id, name, type, config_json, secret_enc, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, in.Name, in.Type, string(cfg), secretEnc, in.Enabled, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("渠道名称已存在")
		}
		return nil, fmt.Errorf("insert channel: %w", err)
	}
	return s.Get(ctx, id)
}

// Update modifies a channel; empty Secret keeps the old value.
func (s *Service) Update(ctx context.Context, id string, in Input) (*Channel, error) {
	existing, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateInput(in); err != nil {
		return nil, err
	}
	secretEnc := existing.SecretEnc
	if in.Secret != "" {
		secretEnc, err = s.saveSecret(id, in.Secret)
		if err != nil {
			return nil, err
		}
	}
	cfg, _ := json.Marshal(channelConfig{URL: strings.TrimSpace(in.URL)})
	_, err = s.db.ExecContext(ctx, `UPDATE notify_channels SET name = ?, type = ?, config_json = ?, secret_enc = ?, enabled = ?, updated_at = ?
		WHERE id = ?`, in.Name, in.Type, string(cfg), secretEnc, in.Enabled, s.nowMs(), id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, invalid("渠道名称已存在")
		}
		return nil, fmt.Errorf("update channel: %w", err)
	}
	return s.Get(ctx, id)
}

// Delete removes a channel.
func (s *Service) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notify_channels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) secret(ctx context.Context, r row) (string, error) {
	if len(r.SecretEnc) == 0 {
		return "", nil
	}
	b, err := s.creds.DecryptSensitive(r.SecretEnc, r.ID)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func hmacBase64(key, msg string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(msg))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Service) buildRequest(ch Channel, secret, text string) (*http.Request, error) {
	var body []byte
	var err error
	target := ch.URL
	switch ch.Type {
	case "webhook":
		body, err = json.Marshal(map[string]any{"text": text})
	case "wecom":
		body, err = json.Marshal(map[string]any{"msgtype": "text", "text": map[string]string{"content": text}})
	case "dingtalk":
		payload := map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}
		if secret != "" {
			ts := strconv.FormatInt(s.now().UnixMilli(), 10)
			sign := hmacBase64(secret, ts+"\n"+secret)
			sep := "?"
			if strings.Contains(target, "?") {
				sep = "&"
			}
			target += sep + "timestamp=" + ts + "&sign=" + url.QueryEscape(sign)
		}
		body, err = json.Marshal(payload)
	case "feishu":
		payload := map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
		if secret != "" {
			ts := strconv.FormatInt(s.now().Unix(), 10)
			sign := hmacBase64(ts+"\n"+secret, "")
			payload["timestamp"] = ts
			payload["sign"] = sign
		}
		body, err = json.Marshal(payload)
	default:
		return nil, fmt.Errorf("unsupported channel type %q", ch.Type)
	}
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	return req, nil
}

// Send posts one message and returns request/response snippets.
func (s *Service) Send(ctx context.Context, ch Channel, text string) (string, string, error) {
	r, err := s.load(ctx, ch.ID)
	if err != nil {
		return "", "", err
	}
	secret, err := s.secret(ctx, r)
	if err != nil {
		return "", "", fmt.Errorf("decrypt channel secret: %w", err)
	}
	req, err := s.buildRequest(ch, secret, text)
	if err != nil {
		return "", "", err
	}
	reqSnippet := req.Method + " " + req.URL.Redacted()
	resp, err := s.client.Do(req.WithContext(ctx))
	if err != nil {
		return reqSnippet, "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	respSnippet := string(respBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return reqSnippet, respSnippet, fmt.Errorf("通知返回 HTTP %d", resp.StatusCode)
	}
	return reqSnippet, respSnippet, nil
}

type scheduleNotifyConfig struct {
	ChannelIDs []string `json:"channel_ids"`
	Events     string   `json:"events"`
}

func (s *Service) format(ex ExecutionNotice) string {
	lines := []string{
		fmt.Sprintf("[Spool] 任务「%s」执行 %s", ex.Name, ex.Status),
		fmt.Sprintf("主机：成功 %d / 失败 %d / 取消 %d / 共 %d", ex.HostSuccess, ex.HostFailed, ex.HostCancelled, ex.HostTotal),
	}
	if ex.FinishedAt != nil {
		lines = append(lines, "结束时间："+time.UnixMilli(*ex.FinishedAt).Format("2006-01-02 15:04:05"))
	}
	if ex.ErrorSummary != "" {
		lines = append(lines, "摘要："+ex.ErrorSummary)
	}
	if s.baseURL != "" {
		lines = append(lines, "详情："+s.baseURL+"/executions/"+ex.ID)
	}
	return strings.Join(lines, "\n")
}

// NotifyExecution sends messages for a finished scheduled execution.
func (s *Service) NotifyExecution(ctx context.Context, ex ExecutionNotice) {
	if ex.ScheduleID == "" {
		return
	}
	var raw struct {
		ChannelIDsJSON string `db:"notify_channel_ids_json"`
		Events         string `db:"notify_events"`
	}
	if err := s.db.GetContext(ctx, &raw, `SELECT notify_channel_ids_json, notify_events FROM schedules WHERE id = ?`, ex.ScheduleID); err != nil {
		return
	}
	ids := []string{}
	_ = json.Unmarshal([]byte(raw.ChannelIDsJSON), &ids)
	if len(ids) == 0 {
		return
	}
	if raw.Events != "always" {
		if ex.Status != "failed" && ex.Status != "partial" {
			return
		}
	}
	text := s.format(ex)
	for _, id := range ids {
		ch, err := s.Get(ctx, id)
		if err != nil || !ch.Enabled {
			continue
		}
		go s.deliver(*ch, text, ex.ID)
	}
}

func (s *Service) deliver(ch Channel, text, execID string) {
	backoff := []time.Duration{time.Second, 5 * time.Second, 25 * time.Second}
	var (
		lastErr  error
		reqSnip  string
		respSnip string
		attempts int
	)
	for attempts = 1; attempts <= 3; attempts++ {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		reqSnip, respSnip, lastErr = s.Send(ctx, ch, text)
		cancel()
		if lastErr == nil {
			break
		}
		if attempts < 3 {
			time.Sleep(backoff[attempts-1])
		}
	}
	status := "success"
	errMsg := ""
	if lastErr != nil {
		status = "failed"
		errMsg = lastErr.Error()
	}
	_, _ = s.db.Exec(`INSERT INTO notify_deliveries (id, execution_id, channel_id, status, attempts, request_snippet, response_snippet, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		store.NewULID(), execID, ch.ID, status, attempts, truncate(reqSnip, 500), truncate(respSnip, 500), errMsg, s.nowMs())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Test sends a sample message and returns the response snippet.
func (s *Service) Test(ctx context.Context, id string) (string, error) {
	ch, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, resp, err := s.Send(ctx, *ch, "[Spool] 这是一条测试通知")
	return resp, err
}
