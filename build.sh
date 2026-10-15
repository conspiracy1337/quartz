#!/usr/bin/env bash
# Builds a stripped production binary into build/bin/quartz.
# Service IDs (Microsoft sign-in, CurseForge, Discord) come from .env or the environment.
# Extra arguments are passed through to `wails build` (e.g. ./build.sh -platform windows/amd64).
set -euo pipefail

cd "$(dirname "$0")"
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

tags=()
# Newer distros ship only WebKitGTK 4.1, which Wails needs to be told about.
if [[ "$(uname)" == "Linux" ]] && ! pkg-config --exists webkit2gtk-4.0 && pkg-config --exists webkit2gtk-4.1; then
  tags=(-tags webkit2_41)
fi

ids=$(go run ./tools/release ldflags)
wails build -clean -trimpath -ldflags "-s -w $ids" "${tags[@]}" "$@"
