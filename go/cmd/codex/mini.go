package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func runRemote(inv miniInvocation) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	session := inv.session
	if session == "" {
		session = os.Getenv("CODEX_REMOTE_SESSION")
	}
	if session == "" {
		session = defaultRemoteSession(cwd, home, os.Getppid())
	}
	session = sanitizeSessionName(session)

	backend, err := ensureRemoteBackend(inv.host, session, cwd)
	if err != nil {
		return err
	}
	endpoint, stopTunnel, err := openRemoteTunnel(inv.host, backend.Socket)
	if err != nil {
		return err
	}
	defer stopTunnel()

	path, err := findRealCodex()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, remoteTUIArgs(inv.args, endpoint, backend.CWD, backend.ThreadID)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()
	stopTunnel()
	if runErr == nil {
		if err := stopRemoteBackend(inv.host, session); err != nil {
			return fmt.Errorf("Codex exited, but Mini backend cleanup failed: %w", err)
		}
		return nil
	}

	threadID := backend.ThreadID
	if threadID == "" {
		threadID, _ = bindRemoteBackend(inv.host, session)
	}
	fmt.Fprintf(os.Stderr, "codex: Mini session kept after disconnect; reconnect with codex --mini --session %s\n", session)
	if threadID != "" {
		fmt.Fprintf(os.Stderr, "codex: remote Codex session %s\n", threadID)
	}
	return fmt.Errorf("remote Codex session %s: %w", session, runErr)
}

func ensureRemoteBackend(host, session, cwd string) (remoteBackend, error) {
	command := shellJoin([]string{"$HOME/.local/bin/codex", "__ensure-mini-backend", session, cwd})
	cmd := exec.Command("ssh", sshArgs(host, command)...)
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return remoteBackend{}, fmt.Errorf("start Mini backend: %w", err)
	}
	var backend remoteBackend
	if err := json.Unmarshal(output, &backend); err != nil {
		return remoteBackend{}, fmt.Errorf("decode Mini backend: %w", err)
	}
	return backend, nil
}

func bindRemoteBackend(host, session string) (string, error) {
	command := shellJoin([]string{"$HOME/.local/bin/codex", "__bind-mini-backend", session})
	cmd := exec.Command("ssh", sshArgs(host, command)...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bind Mini thread: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func stopRemoteBackend(host, session string) error {
	command := shellJoin([]string{"$HOME/.local/bin/codex", "__stop-mini-backend", session})
	cmd := exec.Command("ssh", sshArgs(host, command)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stop Mini backend: %w", err)
	}
	return nil
}

func openRemoteTunnel(host, remoteSocket string) (string, func(), error) {
	dir, err := os.MkdirTemp("/private/tmp", "codex-mini-")
	if err != nil {
		return "", nil, fmt.Errorf("create local socket directory: %w", err)
	}
	localSocket := filepath.Join(dir, "app-server.sock")
	cmd := exec.Command("ssh", sshArgs(
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StreamLocalBindUnlink=yes",
		"-L", localSocket+":"+remoteSocket,
		host,
	)...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("start Mini tunnel: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-done
		}
		_ = os.RemoveAll(dir)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, statErr := os.Stat(localSocket); statErr == nil && info.Mode()&os.ModeSocket != 0 {
			return "unix://" + localSocket, stop, nil
		}
		select {
		case tunnelErr := <-done:
			stopped = true
			_ = os.RemoveAll(dir)
			return "", nil, fmt.Errorf("Mini tunnel exited during startup: %w", tunnelErr)
		case <-time.After(50 * time.Millisecond):
		}
	}
	stop()
	return "", nil, errors.New("timed out waiting for Mini tunnel")
}

func remoteTUIArgs(args []string, endpoint, cwd, threadID string) []string {
	base := []string{"--remote", endpoint, "--no-alt-screen", "-C", cwd}
	if threadID != "" {
		return append(append([]string{"resume"}, base...), append(args, threadID)...)
	}
	if len(args) > 0 && args[0] == "resume" {
		return append(append([]string{"resume"}, base...), args[1:]...)
	}
	return append(base, args...)
}

func defaultRemoteSession(cwd, home string, parentPID int) string {
	sessionPath := strings.TrimPrefix(cwd, home+string(filepath.Separator))
	return sanitizeSessionName("codex-" + strings.ReplaceAll(sessionPath, string(filepath.Separator), "-") + "-" + strconv.Itoa(parentPID))
}

func sshArgs(extra ...string) []string {
	args := []string{"-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"}
	return append(args, extra...)
}

func sanitizeSessionName(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if strings.HasPrefix(arg, "$HOME/") {
			quoted[i] = `"$HOME"/` + shellQuote(strings.TrimPrefix(arg, "$HOME/"))
			continue
		}
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
