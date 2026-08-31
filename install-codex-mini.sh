#!/bin/bash

set -euo pipefail

DOTFILES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v go >/dev/null 2>&1; then
    if ! command -v brew >/dev/null 2>&1; then
        echo "install-codex-mini: Go is required" >&2
        exit 1
    fi
    brew install go
fi

mkdir -p "$HOME/.local/bin" "$HOME/.local/libexec"
target="$HOME/.local/bin/codex"
real="$HOME/.local/libexec/codex-real"

if [[ -x "$target" ]] && ! "$target" __mini-wrapper 2>/dev/null | grep -qx 'codex-mini-wrapper-v1'; then
    if [[ -e "$real" ]]; then
        echo "install-codex-mini: refusing to overwrite $real" >&2
        exit 1
    fi
    mv "$target" "$real"
fi

temporary="$(mktemp "$HOME/.local/bin/.codex-mini.XXXXXX")"
trap 'rm -f "$temporary"' EXIT

(
    cd "$DOTFILES_DIR/go"
    go test ./...
    go build -o "$temporary" ./cmd/codex
)

chmod 0755 "$temporary"
mv "$temporary" "$target"
trap - EXIT

echo "Installed Codex Mini wrapper at $target"
