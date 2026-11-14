#!/usr/bin/env bash
# Installs the pinned dev tools into $(go env GOPATH)/bin. CI and the orb setup
# both run this, so a contributor, an agent and CI check with the same versions.
set -euo pipefail

GOLANGCI_LINT_VERSION=2.15.0
ACTIONLINT_VERSION=1.7.12
SHELLCHECK_VERSION=0.11.0

bin="$(go env GOPATH)/bin"
mkdir -p "$bin"
export PATH="$bin:$PATH"

# have <command> <version-substring>: true when the command reports the version.
have() { command -v "$1" >/dev/null && "$1" version 2>&1 | grep -Fq "$2"; }

echo "== templ"
templ_ver="$(go list -m -f '{{.Version}}' github.com/a-h/templ)"
if ! have templ "$templ_ver"; then
  go install "github.com/a-h/templ/cmd/templ@$templ_ver"
fi

echo "== golangci-lint (lint and gofumpt)"
if ! have golangci-lint "$GOLANGCI_LINT_VERSION"; then
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh |
    sh -s -- -b "$bin" "v$GOLANGCI_LINT_VERSION"
fi

echo "== actionlint"
if ! actionlint -version 2>/dev/null | grep -Fq "$ACTIONLINT_VERSION"; then
  go install "github.com/rhysd/actionlint/cmd/actionlint@v$ACTIONLINT_VERSION"
fi

echo "== shellcheck"
if ! command -v shellcheck >/dev/null; then
  case "$(uname -m)" in aarch64|arm64) arch=aarch64 ;; *) arch=x86_64 ;; esac
  tmp="$(mktemp -d)"
  curl -fsSL "https://github.com/koalaman/shellcheck/releases/download/v$SHELLCHECK_VERSION/shellcheck-v$SHELLCHECK_VERSION.linux.$arch.tar.xz" |
    tar -xJ -C "$tmp"
  cp "$tmp/shellcheck-v$SHELLCHECK_VERSION/shellcheck" "$bin/shellcheck"
  rm -rf "$tmp"
fi
