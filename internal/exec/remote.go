package exec

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// ShellQuote wraps a value in single quotes with POSIX escaping.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// BuildEnvFile renders export statements loaded by run.sh.
func BuildEnvFile(vars map[string]string) []byte {
	names := make([]string, 0, len(vars))
	for k := range vars {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "export %s=%s\n", name, ShellQuote(vars[name]))
	}
	return []byte(b.String())
}

// ScriptFileName returns the remote script name for a tool type.
func ScriptFileName(typ string) string {
	if typ == "python" {
		return "script.py"
	}
	return "script.sh"
}

// BuildRunScript renders run.sh: pid file, env loading and exec.
func BuildRunScript(step Step, scriptName string) []byte {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("set -e\n")
	b.WriteString("echo $$ > .spool.pid\n")
	b.WriteString("set -a\n")
	b.WriteString(". ./.spool-env\n")
	b.WriteString("set +a\n")
	interpreter := step.Interpreter
	if step.Type == "python" {
		interpreter += " -u"
	}
	fmt.Fprintf(&b, "exec %s ./%s\n", interpreter, scriptName)
	return []byte(b.String())
}

// BuildCommand wraps run.sh with sudo/timeout/setsid when available.
func BuildCommand(dir string, step Step, hasSetsid, hasTimeout bool) string {
	prefix := ""
	if step.Sudo {
		prefix = "sudo -n "
	}
	timeout := ""
	if hasTimeout {
		timeout = fmt.Sprintf("timeout -k 10 %d ", step.TimeoutSec)
	}
	setsid := ""
	if hasSetsid {
		setsid = "setsid "
	}
	return fmt.Sprintf("cd %s && exec %s%s%s/bin/sh ./run.sh", ShellQuote(dir), prefix, timeout, setsid)
}

// RemoteDir returns the per-step working directory on the target host.
func RemoteDir(base, stepRunID string) string {
	if base == "" {
		base = "/tmp"
	}
	return path.Join(base, "spool-"+stepRunID)
}

func sudoPrefix(sudo bool) string {
	if sudo {
		return "sudo -n "
	}
	return ""
}

// KillCommand terminates the script's process group (falling back to the pid).
func KillCommand(pid int, sudo bool, force bool) string {
	sig := "TERM"
	if force {
		sig = "KILL"
	}
	p := sudoPrefix(sudo)
	return fmt.Sprintf("%skill -%s -- -%d 2>/dev/null || %skill -%s %d 2>/dev/null || true",
		p, sig, pid, p, sig, pid)
}

// CleanupCommand removes the remote working directory.
func CleanupCommand(dir string, sudo bool) string {
	return sudoPrefix(sudo) + "rm -rf " + ShellQuote(dir) + " 2>/dev/null || true"
}
