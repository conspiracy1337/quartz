#!/usr/bin/env bash
# Runs every check a change must pass before it is merged: formatting, vet,
# lint, Go and frontend tests, a production build and its release checks
# (no development tags, Linux and Windows under the 20 MB limit). Stops at
# the first failure.
#   ./check.sh           offline checks
#   ./check.sh -online   also the integration tests and govulncheck (network)
set -euo pipefail

cd "$(dirname "$0")"
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

online=false
for arg in "$@"; do
  case "$arg" in
    -online) online=true ;;
    *) echo "usage: ./check.sh [-online]" >&2; exit 2 ;;
  esac
done

tags=""
# Newer distros ship only WebKitGTK 4.1, which Wails needs to be told about.
if [[ "$(uname)" == "Linux" ]] && ! pkg-config --exists webkit2gtk-4.0 && pkg-config --exists webkit2gtk-4.1; then
  tags=webkit2_41
fi

step() { printf '\n\033[1m== %s\033[0m\n' "$1"; }

step "gofmt (files changed since main)"
base=$(git merge-base main HEAD 2>/dev/null || git rev-parse HEAD)
mapfile -t changed < <({ git diff --name-only --diff-filter=ACMR "$base" -- '*.go'; git ls-files --others --exclude-standard -- '*.go'; } | sort -u)
if ((${#changed[@]})); then
  unformatted=$(gofmt -l "${changed[@]}")
  if [[ -n "$unformatted" ]]; then
    echo "not gofmt-clean:"; echo "$unformatted"; exit 1
  fi
fi

step "go vet"
go vet -tags "$tags" ./...

step "golangci-lint"
golangci-lint run --build-tags "$tags" ./...

step "go test"
go test -tags "$tags" ./...

step "frontend: svelte-check"
(cd frontend && pnpm run --silent check)

step "frontend: vitest"
(cd frontend && pnpm run --silent test)

step "production build"
./build.sh >/dev/null
go run ./tools/release verify build/bin/quartz

# Windows builds come out about 1 MB larger (PE relocations and unwind data,
# WebView2 bindings), so the limit is checked there too; the frontend is
# reused from the build above.
step "windows build"
./build.sh -platform windows/amd64 -s -o quartz.exe >/dev/null
go run ./tools/release verify build/bin/quartz.exe

if $online; then
  step "integration tests"
  go test -tags "integration${tags:+,$tags}" ./internal/integration/ ./internal/core/docs/

  step "govulncheck"
  govulncheck -tags "$tags" ./...
fi

step "all checks passed"
