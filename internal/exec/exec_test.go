package exec

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveExecutionStatus(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"success", "success"}, "success"},
		{[]string{"success", "failed"}, "partial"},
		{[]string{"failed", "failed"}, "failed"},
		{[]string{"success", "cancelled"}, "cancelled"},
		{[]string{}, "success"},
	}
	for _, c := range cases {
		if got := DeriveExecutionStatus(c.in); got != c.want {
			t.Fatalf("%v => %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDeriveHostRunStatus(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"success", "success"}, "success"},
		{[]string{"success", "skipped"}, "success"},
		{[]string{"success", "failed"}, "failed"},
		{[]string{"cancelled", "failed"}, "cancelled"},
		{[]string{"queued"}, "failed"},
	}
	for _, c := range cases {
		if got := DeriveHostRunStatus(c.in); got != c.want {
			t.Fatalf("%v => %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("a'b"); got != `'a'\''b'` {
		t.Fatalf("quote = %s", got)
	}
}

func TestBuildEnvFile(t *testing.T) {
	b := string(BuildEnvFile(map[string]string{"A": "x y", "B": "it's", "C": ""}))
	if !strings.Contains(b, `export A='x y'`) {
		t.Fatalf("env = %s", b)
	}
	if !strings.Contains(b, `export B='it'\''s'`) {
		t.Fatalf("env = %s", b)
	}
	if !strings.Contains(b, `export C=''`) {
		t.Fatalf("env = %s", b)
	}
}

func TestBuildRunScript(t *testing.T) {
	step := Step{Type: "python", Interpreter: "python3", PrepareCommand: "pip install x", TimeoutSec: 30}
	b := string(BuildRunScript(step, "script.py"))
	for _, want := range []string{"echo $$ > .spool.pid", ". ./.spool-env", "pip install x", "exec python3 ./script.py"} {
		if !strings.Contains(b, want) {
			t.Fatalf("script missing %q:\n%s", want, b)
		}
	}
}

func TestBuildCommand(t *testing.T) {
	step := Step{Type: "shell", Interpreter: "/bin/bash", Sudo: true, TimeoutSec: 30}
	cmd := BuildCommand("/tmp/spool-1", step, true, true)
	if !strings.Contains(cmd, "sudo -n timeout -k 10 30 setsid /bin/sh ./run.sh") {
		t.Fatalf("cmd = %s", cmd)
	}
	if !strings.HasPrefix(cmd, "cd '/tmp/spool-1' && exec ") {
		t.Fatalf("cmd = %s", cmd)
	}
	plain := BuildCommand("/tmp/spool-1", Step{TimeoutSec: 5}, false, false)
	if !strings.Contains(plain, "exec /bin/sh ./run.sh") || strings.Contains(plain, "setsid") {
		t.Fatalf("plain = %s", plain)
	}
}

func TestReadEventsReplayAndLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e.jsonl")
	log, err := OpenEventLog(path, "e1", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		log.Emit(Event{Type: "log", Stream: "stdout", Data: "line"})
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	events, err := ReadEvents(path, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 2 {
		t.Fatalf("events = %+v", events)
	}
	events, _ = ReadEvents(path, 0, 0)
	if len(events) != 3 {
		t.Fatalf("len = %d", len(events))
	}
}

func TestRenderText(t *testing.T) {
	events := []Event{
		{Type: "log", HostRunID: "h1", StepRunID: "s1", Stream: "stdout", Data: "a\n"},
		{Type: "log", HostRunID: "h1", StepRunID: "s1", Stream: "stderr", Data: "b"},
		{Type: "state", HostRunID: "h1"},
	}
	out := RenderText(events, "h1", "s1")
	if !strings.Contains(out, "a") || !strings.Contains(out, "stderr") || !strings.Contains(out, "b\n") {
		t.Fatalf("out = %q", out)
	}
}
