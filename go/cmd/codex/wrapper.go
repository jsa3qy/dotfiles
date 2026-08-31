package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	defaultRemoteHost = "mini"
	wrapperMarker     = "codex-mini-wrapper-v1"
)

var miniCodexArgs = []string{
	"-c", "shell_environment_policy.inherit=all",
	"-c", "shell_environment_policy.ignore_default_excludes=true",
}

type miniInvocation struct {
	host    string
	session string
	args    []string
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "__mini-wrapper" {
		fmt.Fprintln(os.Stdout, wrapperMarker)
		return nil
	}
	if len(args) > 0 {
		switch args[0] {
		case "__ensure-mini-backend":
			if len(args) != 3 {
				return errors.New("invalid Mini backend invocation")
			}
			backend, err := ensureBackend(args[1], args[2])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(backend)
		case "__bind-mini-backend":
			if len(args) != 2 {
				return errors.New("invalid Mini backend bind invocation")
			}
			threadID, err := bindBackend(args[1])
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stdout, threadID)
			return nil
		case "__stop-mini-backend":
			if len(args) != 2 {
				return errors.New("invalid Mini backend stop invocation")
			}
			return stopBackend(args[1])
		case "__run-mini-app-server":
			if len(args) != 2 {
				return errors.New("invalid Mini app-server invocation")
			}
			serverArgs := append(append([]string{}, miniCodexArgs...), "app-server", "--listen", "unix://"+args[1])
			return execRealCodex(serverArgs)
		}
	}

	inv, remote, err := parseMiniInvocation(args)
	if err != nil {
		return err
	}
	if !remote {
		return execRealCodex(args)
	}
	return runRemote(inv)
}

func parseMiniInvocation(args []string) (miniInvocation, bool, error) {
	remote := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--mini" {
			remote = true
		}
	}
	if !remote {
		return miniInvocation{}, false, nil
	}

	inv := miniInvocation{host: os.Getenv("CODEX_REMOTE_HOST")}
	if inv.host == "" {
		inv.host = defaultRemoteHost
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mini":
		case "--session":
			if i+1 >= len(args) || args[i+1] == "" {
				return miniInvocation{}, false, errors.New("--session requires a name")
			}
			inv.session = args[i+1]
			i++
		default:
			inv.args = append(inv.args, args[i])
		}
	}
	return inv, true, nil
}

func execRealCodex(args []string) error {
	path, err := findRealCodex()
	if err != nil {
		return err
	}
	return syscall.Exec(path, append([]string{path}, args...), os.Environ())
}

func findRealCodex() (string, error) {
	if configured := os.Getenv("CODEX_REAL_BIN"); configured != "" {
		return configured, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		preserved := filepath.Join(home, ".local", "libexec", "codex-real")
		if info, statErr := os.Stat(preserved); statErr == nil && info.Mode()&0o111 != 0 {
			return preserved, nil
		}
	}

	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate wrapper executable: %w", err)
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return "", fmt.Errorf("inspect wrapper executable: %w", err)
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		candidate := filepath.Join(dir, "codex")
		info, statErr := os.Stat(candidate)
		if statErr != nil || info.Mode()&0o111 == 0 || os.SameFile(selfInfo, info) {
			continue
		}
		return candidate, nil
	}
	return "", errors.New("real Codex executable not found; set CODEX_REAL_BIN")
}
