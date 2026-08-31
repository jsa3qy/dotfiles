package main

import (
	"reflect"
	"testing"
)

func TestParseMiniInvocation(t *testing.T) {
	inv, remote, err := parseMiniInvocation([]string{"--mini", "--session", "review", "--model", "gpt-5.6-sol"})
	if err != nil || !remote {
		t.Fatalf("parseMiniInvocation() remote=%v err=%v", remote, err)
	}
	if inv.session != "review" || !reflect.DeepEqual(inv.args, []string{"--model", "gpt-5.6-sol"}) {
		t.Fatalf("parseMiniInvocation() = %#v", inv)
	}
}

func TestRemoteTUIArgs(t *testing.T) {
	got := remoteTUIArgs(nil, "unix:///tmp/app.sock", "/repo", "019fa904-7f72-7a90-a7a2-6838187d3c75")
	want := []string{"resume", "--remote", "unix:///tmp/app.sock", "--no-alt-screen", "-C", "/repo", "019fa904-7f72-7a90-a7a2-6838187d3c75"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("remoteTUIArgs() = %q, want %q", got, want)
	}
}

func TestDefaultRemoteSession(t *testing.T) {
	got := defaultRemoteSession("/Users/jessealloy/Beta/entract", "/Users/jessealloy", 1234)
	if got != "codex-Beta-entract-1234" {
		t.Fatalf("defaultRemoteSession() = %q", got)
	}
}

func TestThreadIDFromLsof(t *testing.T) {
	output := []byte("p123\nn/Users/jessealloy/.codex/sessions/2026/08/rollout-019fa904-7f72-7a90-a7a2-6838187d3c75.jsonl\n")
	if got := threadIDFromLsof(output); got != "019fa904-7f72-7a90-a7a2-6838187d3c75" {
		t.Fatalf("threadIDFromLsof() = %q", got)
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"$HOME/.local/bin/codex", "argument with spaces", "it's"})
	want := `"$HOME"/'.local/bin/codex' 'argument with spaces' 'it'"'"'s'`
	if got != want {
		t.Fatalf("shellJoin() = %q, want %q", got, want)
	}
}
