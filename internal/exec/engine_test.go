package exec

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/store"
)

type fakeResponse struct {
	code    int
	stdout  string
	stderr  string
	err     error
	block   bool
	sleep   time.Duration
	hasCode bool
	codeVal int
}

type fakeRunner struct {
	mu            sync.Mutex
	uploads       map[string]string
	runCmds       []string
	cleanups      int
	kills         int
	responses     []fakeResponse
	concurrent    int32
	maxConcurrent int32
}

func (f *fakeRunner) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return []byte("4242"), nil
}

func (f *fakeRunner) Close() error { return nil }

func (f *fakeRunner) run(ctx context.Context, cmd string, onOut, onErr func([]byte)) (int, error) {
	if strings.Contains(cmd, "run.sh") {
		cur := atomic.AddInt32(&f.concurrent, 1)
		for {
			max := atomic.LoadInt32(&f.maxConcurrent)
			if cur <= max || atomic.CompareAndSwapInt32(&f.maxConcurrent, max, cur) {
				break
			}
		}
		defer atomic.AddInt32(&f.concurrent, -1)
		f.mu.Lock()
		var r fakeResponse
		if len(f.responses) > 0 {
			r = f.responses[0]
			f.responses = f.responses[1:]
		}
		f.runCmds = append(f.runCmds, cmd)
		f.mu.Unlock()
		if r.sleep > 0 {
			time.Sleep(r.sleep)
		}
		if r.block {
			<-ctx.Done()
			return -1, ctx.Err()
		}
		if r.stdout != "" && onOut != nil {
			onOut([]byte(r.stdout))
		}
		if r.stderr != "" && onErr != nil {
			onErr([]byte(r.stderr))
		}
		if r.err != nil {
			return -1, r.err
		}
		return r.code, nil
	}
	f.mu.Lock()
	if strings.Contains(cmd, "rm -rf") {
		f.cleanups++
	}
	if strings.Contains(cmd, "kill ") {
		f.kills++
	}
	f.mu.Unlock()
	return 0, nil
}

type fakeRunnerWrapper struct {
	r *fakeRunner
}

func (w *fakeRunnerWrapper) Upload(ctx context.Context, path string, data []byte, mode fs.FileMode) error {
	w.r.mu.Lock()
	if w.r.uploads == nil {
		w.r.uploads = map[string]string{}
	}
	w.r.uploads[path] = string(data)
	w.r.mu.Unlock()
	return nil
}

func (w *fakeRunnerWrapper) RunStream(ctx context.Context, cmd string, onOut, onErr func([]byte)) (int, error) {
	return w.r.run(ctx, cmd, onOut, onErr)
}

func (w *fakeRunnerWrapper) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return w.r.ReadFile(ctx, path)
}

func (w *fakeRunnerWrapper) Close() error { return nil }

type fakeFactory struct {
	runner Runner
	err    error
}

func (f *fakeFactory) Connect(ctx context.Context, h HostRef) (Runner, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.runner, nil
}

type engineFixture struct {
	engine *Engine
	store  *store.Store
	creds  *credential.Service
	hostID string
	logs   string
}

func newEngineFixture(t *testing.T, cfg Config, runner Runner) *engineFixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cipher, err := credential.NewCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	creds := credential.NewService(st.DB, cipher)
	cred, err := creds.Create(context.Background(), credential.Input{
		Name: "c", AuthType: "password", Username: "root", Secret: "pw",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	hostID := store.NewULID()
	capJSON := `{"bash_path":"/bin/bash","python_path":"/usr/bin/python3","setsid":false,"timeout":false,"probed_at":1}`
	_, err = st.DB.Exec(`INSERT INTO hosts (id, name, address, port, credential_id, enabled, capability_json, created_at, updated_at)
		VALUES (?, 'h1', '127.0.0.1', 22, ?, 1, ?, 1, 1)`, hostID, cred.ID, capJSON)
	if err != nil {
		t.Fatal(err)
	}
	logs := t.TempDir()
	cfg.LogsDir = logs
	eng := New(st.DB, creds, &fakeFactory{runner: runner}, cfg)
	if err := eng.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(eng.Stop)
	return &engineFixture{engine: eng, store: st, creds: creds, hostID: hostID, logs: logs}
}

func toolPlan(name, content string) Plan {
	return BuildToolPlan(name, ToolVersionSnapshot{
		ToolID: "tool-1", VersionID: "ver-1", VersionNo: 1,
		Type: "shell", Interpreter: "/bin/bash", Content: content, TimeoutSec: 5,
	}, nil, nil)
}

func waitTerminal(t *testing.T, e *Engine, id string) *Execution {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ex, err := e.GetExecution(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		switch ex.Status {
		case "success", "partial", "failed", "cancelled", "interrupted":
			return ex
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("execution did not reach a terminal status")
	return nil
}

func TestExecutionSuccess(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{code: 0, stdout: "hello\n", stderr: "warn\n"}}}
	fx := newEngineFixture(t, Config{MaxConcurrentSteps: 2}, &fakeRunnerWrapper{runner})
	ex, err := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "demo", Plan: toolPlan("demo", "echo hello"),
		Params: map[string]string{"ENV": "prod"}, Sensitive: map[string]string{"TOKEN": "s3cr3t"},
		HostIDs: []string{fx.hostID}, ActorID: "admin",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "success" {
		t.Fatalf("status = %q, summary = %q", final.Status, final.ErrorSummary)
	}
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if len(steps) != 1 || steps[0].Status != "success" || steps[0].LogBytes == 0 {
		t.Fatalf("steps = %+v", steps)
	}
	if runner.cleanups == 0 {
		t.Fatal("remote dir was not cleaned")
	}
	sensitive, err := fx.engine.DecryptSensitive(final)
	if err != nil || sensitive["TOKEN"] != "s3cr3t" {
		t.Fatalf("sensitive = %v err=%v", sensitive, err)
	}
	events, err := ReadEvents(filepath.Join(fx.logs, ex.ID+".jsonl"), 0, 0)
	if err != nil || len(events) == 0 {
		t.Fatalf("events = %d err=%v", len(events), err)
	}
	foundOut, foundErr := false, false
	for _, ev := range events {
		if ev.Type == "log" && ev.Stream == "stdout" && strings.Contains(ev.Data, "hello") {
			foundOut = true
		}
		if ev.Type == "log" && ev.Stream == "stderr" && strings.Contains(ev.Data, "warn") {
			foundErr = true
		}
	}
	if !foundOut || !foundErr {
		t.Fatalf("missing log events: %+v", events)
	}
}

func TestExitCodeFailure(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{code: 3, stdout: "boom\n"}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{runner})
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "demo", Plan: toolPlan("demo", "exit 3"), HostIDs: []string{fx.hostID},
	})
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "failed" {
		t.Fatalf("status = %q", final.Status)
	}
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if steps[0].FailureReason != "exit_code" || steps[0].ExitCode == nil || *steps[0].ExitCode != 3 {
		t.Fatalf("step = %+v", steps[0])
	}
}

func TestRetryThenSuccess(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{code: 1, stdout: "first\n"}, {code: 0, stdout: "second\n"}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{runner})
	plan := toolPlan("demo", "run")
	plan.Stages[0].Steps[0].RetryCount = 1
	plan.Stages[0].Steps[0].RetryDelaySec = 1
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "demo", Plan: plan, HostIDs: []string{fx.hostID},
	})
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "success" {
		t.Fatalf("status = %q", final.Status)
	}
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if steps[0].Attempt != 2 {
		t.Fatalf("attempt = %d", steps[0].Attempt)
	}
}

func TestTimeoutFails(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{block: true}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{runner})
	plan := toolPlan("demo", "sleep 100")
	plan.Stages[0].Steps[0].TimeoutSec = 1
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "demo", Plan: plan, HostIDs: []string{fx.hostID},
	})
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "failed" {
		t.Fatalf("status = %q", final.Status)
	}
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if steps[0].FailureReason != "timeout" {
		t.Fatalf("reason = %q", steps[0].FailureReason)
	}
}

func TestCancelExecution(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{block: true}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{runner})
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "demo", Plan: toolPlan("demo", "sleep"), HostIDs: []string{fx.hostID},
	})
	time.Sleep(100 * time.Millisecond)
	if err := fx.engine.Cancel(context.Background(), ex.ID); err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "cancelled" {
		t.Fatalf("status = %q", final.Status)
	}
}

func TestOutputTruncation(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{stdout: "0123456789"}}}
	fx := newEngineFixture(t, Config{OutputLimitBytes: 5}, &fakeRunnerWrapper{runner})
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "demo", Plan: toolPlan("demo", "print"), HostIDs: []string{fx.hostID},
	})
	waitTerminal(t, fx.engine, ex.ID)
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if !steps[0].Truncated || steps[0].LogBytes != 5 || steps[0].DroppedBytes != 5 {
		t.Fatalf("step = %+v", steps[0])
	}
}

func TestStepConcurrencyLimit(t *testing.T) {
	runner := &fakeRunner{}
	for i := 0; i < 2; i++ {
		runner.responses = append(runner.responses, fakeResponse{sleep: 80 * time.Millisecond})
	}
	fx := newEngineFixture(t, Config{MaxConcurrentSteps: 1}, &fakeRunnerWrapper{runner})
	plan := Plan{Kind: "tool", Name: "multi", Stages: []Stage{{
		Index: 1,
		Steps: []Step{
			{Index: 1, Name: "a", Type: "shell", Interpreter: "/bin/bash", Content: "a", TimeoutSec: 5},
			{Index: 2, Name: "b", Type: "shell", Interpreter: "/bin/bash", Content: "b", TimeoutSec: 5},
		},
	}}}
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "multi", Plan: plan, HostIDs: []string{fx.hostID},
	})
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "success" {
		t.Fatalf("status = %q", final.Status)
	}
	if got := atomic.LoadInt32(&runner.maxConcurrent); got != 1 {
		t.Fatalf("max concurrent = %d, want 1", got)
	}
}

func TestStartupMarksInterrupted(t *testing.T) {
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: &fakeRunner{}})
	now := time.Now().UnixMilli()
	_, err := fx.store.DB.Exec(`INSERT INTO executions (id, kind, name_snapshot, plan_json, host_total, status, created_at)
		VALUES ('e-old', 'tool', 'old', '{}', 1, 'running', ?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fx.store.DB.Exec(`INSERT INTO host_runs (id, execution_id, host_id, host_name_snapshot, status) VALUES ('hr-old', 'e-old', 'h', 'h', 'running')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fx.store.DB.Exec(`INSERT INTO step_runs (id, host_run_id, status) VALUES ('sr-old', 'hr-old', 'running')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.engine.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = fx.store.DB.Get(&status, `SELECT status FROM executions WHERE id='e-old'`)
	if status != "interrupted" {
		t.Fatalf("execution status = %q", status)
	}
	_ = fx.store.DB.Get(&status, `SELECT status FROM step_runs WHERE id='sr-old'`)
	if status != "interrupted" {
		t.Fatalf("step status = %q", status)
	}
}

func TestSubmitRejectsNoEnabledHosts(t *testing.T) {
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: &fakeRunner{}})
	_, err := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Plan: toolPlan("x", "x"), HostIDs: []string{"missing"},
	})
	if !errors.Is(err, ErrNoHosts) {
		t.Fatalf("err = %v, want ErrNoHosts", err)
	}
}

func TestHostConnectFailureSkipsSteps(t *testing.T) {
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: &fakeRunner{}})
	fx.engine.factory = &fakeFactory{err: errors.New("boom")}
	ex, err := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "tool", Name: "demo", Plan: toolPlan("demo", "echo"), HostIDs: []string{fx.hostID},
	})
	if err != nil {
		t.Fatal(err)
	}
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "failed" || final.FailureReason != "ssh_error" {
		t.Fatalf("execution = %+v", final)
	}
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if len(steps) != 1 || steps[0].Status != "skipped" || steps[0].FailureReason != "skipped" {
		t.Fatalf("step = %+v", steps[0])
	}
}

func TestSensitiveParamsInjectedOnlyWhenReferenced(t *testing.T) {
	runner := &fakeRunner{}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: runner})
	plan := Plan{Kind: "flow", Name: "f", Stages: []Stage{{Index: 1, Steps: []Step{{
		Index: 1, Name: "step", Type: "shell", Interpreter: "/bin/bash", Content: "echo",
		TimeoutSec: 5, Params: map[string]string{"PLAIN": "visible"},
		SensitiveParams: map[string]string{"TOKEN": "flow.secret"},
	}}}}}
	ex, err := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "flow", Name: "f", Plan: plan, HostIDs: []string{fx.hostID},
		Sensitive: map[string]string{"flow.secret": "s3cr3t", "unused": "nope"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if final := waitTerminal(t, fx.engine, ex.ID); final.Status != "success" {
		t.Fatalf("status = %q", final.Status)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	var env string
	for path, content := range runner.uploads {
		if strings.HasSuffix(path, ".spool-env") {
			env = content
		}
	}
	if !strings.Contains(env, "export TOKEN='s3cr3t'") || !strings.Contains(env, "export PLAIN='visible'") {
		t.Fatalf("env = %s", env)
	}
	if strings.Contains(env, "nope") {
		t.Fatalf("unreferenced sensitive value injected: %s", env)
	}
}

func flowPlan(policy string, alwaysRunSecond bool, maxDuration int) Plan {
	return Plan{Kind: "flow", Name: "f", FailurePolicy: policy, MaxDurationSec: maxDuration, Stages: []Stage{
		{Index: 1, Steps: []Step{{Index: 1, Name: "first", Type: "shell", Interpreter: "/bin/bash", Content: "a", TimeoutSec: 5}}},
		{Index: 2, AlwaysRun: alwaysRunSecond, Steps: []Step{{Index: 1, Name: "second", Type: "shell", Interpreter: "/bin/bash", Content: "b", TimeoutSec: 5}}},
	}}
}

func TestFlowAbortRunsAlwaysRunStage(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{code: 1}, {code: 0, stdout: "cleanup\n"}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: runner})
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "flow", Name: "f", Plan: flowPlan("abort", true, 600), HostIDs: []string{fx.hostID},
	})
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "failed" {
		t.Fatalf("status = %q", final.Status)
	}
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if len(steps) != 2 || steps[0].Status != "failed" || steps[1].Status != "success" {
		t.Fatalf("steps = %+v", steps)
	}
	if len(runner.responses) != 0 {
		t.Fatalf("always_run stage did not execute")
	}
}

func TestFlowAbortSkipsNextStage(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{code: 1}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: runner})
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "flow", Name: "f", Plan: flowPlan("abort", false, 600), HostIDs: []string{fx.hostID},
	})
	waitTerminal(t, fx.engine, ex.ID)
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if len(steps) != 2 || steps[1].Status != "skipped" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestFlowContinueRunsNextStage(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{code: 1}, {code: 0}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: runner})
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "flow", Name: "f", Plan: flowPlan("continue", false, 600), HostIDs: []string{fx.hostID},
	})
	waitTerminal(t, fx.engine, ex.ID)
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if len(steps) != 2 || steps[1].Status != "success" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestFlowMaxDuration(t *testing.T) {
	runner := &fakeRunner{responses: []fakeResponse{{block: true}}}
	fx := newEngineFixture(t, Config{}, &fakeRunnerWrapper{r: runner})
	ex, _ := fx.engine.Submit(context.Background(), SubmitRequest{
		Kind: "flow", Name: "f", Plan: flowPlan("continue", true, 1), HostIDs: []string{fx.hostID},
	})
	final := waitTerminal(t, fx.engine, ex.ID)
	if final.Status != "failed" {
		t.Fatalf("status = %q", final.Status)
	}
	hosts, _ := fx.engine.HostRuns(context.Background(), ex.ID)
	if len(hosts) != 1 || hosts[0].FailureReason != "flow_timeout" {
		t.Fatalf("host = %+v", hosts[0])
	}
	steps, _ := fx.engine.StepRuns(context.Background(), ex.ID)
	if steps[0].FailureReason != "timeout" {
		t.Fatalf("step = %+v", steps[0])
	}
}
