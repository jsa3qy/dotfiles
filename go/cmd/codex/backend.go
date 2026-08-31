package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var sessionIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

type remoteBackend struct {
	Session  string `json:"session"`
	Socket   string `json:"socket"`
	CWD      string `json:"cwd"`
	ThreadID string `json:"thread_id,omitempty"`
}

func ensureBackend(session, cwd string) (remoteBackend, error) {
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return remoteBackend{}, fmt.Errorf("remote cwd is unavailable: %s", cwd)
	}
	session = sanitizeSessionName(session)
	tmux, err := findTmux()
	if err != nil {
		return remoteBackend{}, err
	}
	if hasTmuxSession(tmux, session) {
		socket, err := tmuxOption(tmux, session, "@codex_socket")
		if err != nil || socket == "" {
			return remoteBackend{}, fmt.Errorf("tmux session %s is not a Codex Mini backend", session)
		}
		backendCWD, _ := tmuxOption(tmux, session, "@codex_cwd")
		if backendCWD == "" {
			backendCWD = cwd
		}
		_ = setTmuxOption(tmux, session, "@codex_last_connected", strconv.FormatInt(time.Now().Unix(), 10))
		threadID, _ := resolveBackendThread(tmux, session)
		return remoteBackend{Session: session, Socket: socket, CWD: backendCWD, ThreadID: threadID}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return remoteBackend{}, fmt.Errorf("get home directory: %w", err)
	}
	socketDir := filepath.Join(home, ".codex", "remote")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		return remoteBackend{}, fmt.Errorf("create remote socket directory: %w", err)
	}
	socket := filepath.Join(socketDir, session+".sock")
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return remoteBackend{}, fmt.Errorf("remove stale remote socket: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return remoteBackend{}, fmt.Errorf("locate Mini wrapper: %w", err)
	}
	command := shellJoin([]string{self, "__run-mini-app-server", socket})
	if output, startErr := exec.Command(tmux, "new-session", "-d", "-s", session, "-c", cwd, "/bin/zsh", "-lic", command).CombinedOutput(); startErr != nil {
		return remoteBackend{}, fmt.Errorf("start Mini app server: %s", strings.TrimSpace(string(output)))
	}

	started := strconv.FormatInt(time.Now().UnixMilli(), 10)
	for _, option := range [][2]string{
		{"@codex_mini_wrapper", "1"},
		{"@codex_socket", socket},
		{"@codex_cwd", cwd},
		{"@codex_started", started},
		{"@codex_last_connected", strconv.FormatInt(time.Now().Unix(), 10)},
	} {
		if err := setTmuxOption(tmux, session, option[0], option[1]); err != nil {
			_ = exec.Command(tmux, "kill-session", "-t", session).Run()
			_ = os.Remove(socket)
			return remoteBackend{}, err
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, statErr := os.Stat(socket); statErr == nil && info.Mode()&os.ModeSocket != 0 {
			return remoteBackend{Session: session, Socket: socket, CWD: cwd}, nil
		}
		if !hasTmuxSession(tmux, session) {
			output, _ := exec.Command(tmux, "capture-pane", "-p", "-S", "-", "-t", session).CombinedOutput()
			return remoteBackend{}, fmt.Errorf("Mini app server exited during startup: %s", strings.TrimSpace(string(output)))
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = exec.Command(tmux, "kill-session", "-t", session).Run()
	_ = os.Remove(socket)
	return remoteBackend{}, errors.New("timed out waiting for Mini app server")
}

func bindBackend(session string) (string, error) {
	tmux, err := findTmux()
	if err != nil {
		return "", err
	}
	session = sanitizeSessionName(session)
	if !hasTmuxSession(tmux, session) {
		return "", fmt.Errorf("Mini backend %s not found", session)
	}
	threadID, err := resolveBackendThread(tmux, session)
	if err != nil || threadID == "" {
		return threadID, err
	}
	if err := setTmuxOption(tmux, session, "@codex_thread_id", threadID); err != nil {
		return "", fmt.Errorf("record Mini thread: %w", err)
	}
	return threadID, nil
}

func stopBackend(session string) error {
	tmux, err := findTmux()
	if err != nil {
		return err
	}
	session = sanitizeSessionName(session)
	if !hasTmuxSession(tmux, session) {
		return nil
	}
	socket, _ := tmuxOption(tmux, session, "@codex_socket")
	if socket == "" {
		return fmt.Errorf("tmux session %s is not a Codex Mini backend", session)
	}
	if pid, pidErr := appServerPID(socket); pidErr == nil {
		if process, findErr := os.FindProcess(pid); findErr == nil {
			_ = process.Signal(os.Interrupt)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && hasTmuxSession(tmux, session) {
		time.Sleep(100 * time.Millisecond)
	}
	if hasTmuxSession(tmux, session) {
		if output, killErr := exec.Command(tmux, "kill-session", "-t", session).CombinedOutput(); killErr != nil {
			return fmt.Errorf("stop Mini app server: %s", strings.TrimSpace(string(output)))
		}
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove Mini app-server socket: %w", err)
	}
	return nil
}

func resolveBackendThread(tmux, session string) (string, error) {
	if saved, _ := tmuxOption(tmux, session, "@codex_thread_id"); saved != "" {
		return saved, nil
	}
	panePIDText, err := tmuxValue(tmux, session, "#{pane_pid}")
	if err != nil {
		return "", err
	}
	panePID, err := strconv.Atoi(panePIDText)
	if err != nil {
		return "", fmt.Errorf("parse Mini backend PID: %w", err)
	}
	children, err := processChildren()
	if err != nil {
		return "", err
	}
	pids := descendantPIDs(panePID, children)
	values := make([]string, len(pids))
	for i, pid := range pids {
		values[i] = strconv.Itoa(pid)
	}
	lsof, err := findLsof()
	if err != nil {
		return "", err
	}
	output, _ := exec.Command(lsof, "-Fn", "-p", strings.Join(values, ",")).CombinedOutput()
	if threadID := threadIDFromLsof(output); threadID != "" {
		return threadID, nil
	}
	return threadCreatedByBackend(tmux, session), nil
}

func threadCreatedByBackend(tmux, session string) string {
	cwd, _ := tmuxOption(tmux, session, "@codex_cwd")
	started, _ := tmuxOption(tmux, session, "@codex_started")
	if cwd == "" || started == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	query := fmt.Sprintf(
		"SELECT id FROM threads WHERE archived = 0 AND cwd = '%s' AND created_at_ms >= %s ORDER BY created_at_ms LIMIT 2;",
		strings.ReplaceAll(cwd, "'", "''"),
		started,
	)
	output, err := exec.Command("/usr/bin/sqlite3", filepath.Join(codexHome, "state_5.sqlite"), query).Output()
	if err != nil {
		return ""
	}
	ids := strings.Fields(string(output))
	if len(ids) != 1 || !sessionIDPattern.MatchString(ids[0]) {
		return ""
	}
	return strings.ToLower(ids[0])
}

func processChildren() (map[int][]int, error) {
	output, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil, fmt.Errorf("inspect Mini processes: %w", err)
	}
	children := make(map[int][]int)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])
		if pidErr == nil && parentErr == nil {
			children[parent] = append(children[parent], pid)
		}
	}
	return children, nil
}

func descendantPIDs(root int, children map[int][]int) []int {
	result := []int{root}
	for i := 0; i < len(result); i++ {
		result = append(result, children[result[i]]...)
	}
	return result
}

func threadIDFromLsof(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "n") || !strings.Contains(line, string(filepath.Separator)+"sessions"+string(filepath.Separator)) || !strings.HasSuffix(line, ".jsonl") {
			continue
		}
		if sessionID := sessionIDPattern.FindString(line); sessionID != "" {
			return strings.ToLower(sessionID)
		}
	}
	return ""
}

func appServerPID(socket string) (int, error) {
	lsof, err := findLsof()
	if err != nil {
		return 0, err
	}
	output, err := exec.Command(lsof, "-t", socket).Output()
	if err != nil {
		return 0, fmt.Errorf("locate Mini app server: %w", err)
	}
	values := strings.Fields(string(output))
	if len(values) == 0 {
		return 0, errors.New("Mini app-server process not found")
	}
	return strconv.Atoi(values[0])
}

func findTmux() (string, error) {
	for _, candidate := range []string{"tmux", "/opt/homebrew/bin/tmux", "/usr/local/bin/tmux"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("tmux is not installed")
}

func findLsof() (string, error) {
	for _, candidate := range []string{"lsof", "/usr/sbin/lsof", "/usr/bin/lsof"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("lsof is not installed")
}

func hasTmuxSession(tmux, session string) bool {
	return exec.Command(tmux, "has-session", "-t", session).Run() == nil
}

func setTmuxOption(tmux, session, key, value string) error {
	if output, err := exec.Command(tmux, "set-option", "-t", session, key, value).CombinedOutput(); err != nil {
		return fmt.Errorf("configure Mini backend: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func tmuxOption(tmux, session, option string) (string, error) {
	return tmuxValue(tmux, session, "#{"+option+"}")
}

func tmuxValue(tmux, session, format string) (string, error) {
	output, err := exec.Command(tmux, "display-message", "-p", "-t", session, format).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect Mini backend %s: %s", session, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
