#!/usr/bin/env bash
# Builds stripped production binaries for Linux and Windows into build/bin/
# (quartz and quartz.exe), so one run gives both.
# Service IDs (Microsoft sign-in, CurseForge, Discord) come from .env or the environment.
# Extra arguments are passed through to every `wails build` (e.g. ./build.sh -upx).
set -euo pipefail

cd "$(dirname "$0")"
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

# Editing on Windows can turn tracked files' line endings into CRLF, which git
# then reports as changed. Put them back to LF. Only files the repository
# stores as LF are touched, so deliberately mixed test data is left alone.
while IFS= read -r -d '' entry; do
  if [[ $entry == "i/lf "*" w/crlf "* ]]; then
    sed -i 's/\r$//' "${entry#*$'\t'}"
  fi
done < <(git ls-files --eol -z)

ids=$(go run ./tools/release ldflags)
flags=(-trimpath -ldflags "-s -w $ids")

linux_tags=()
# Newer distros ship only WebKitGTK 4.1, which Wails needs to be told about.
# Only the Linux build links against WebKitGTK; Windows never needs the tag.
if [[ "$(uname)" == "Linux" ]] && ! pkg-config --exists webkit2gtk-4.0 && pkg-config --exists webkit2gtk-4.1; then
  linux_tags=(-tags webkit2_41)
fi

# Linux first: a full build that compiles the frontend and cleans build/bin.
go run ./tools/release wails build -clean -platform linux/amd64 "${flags[@]}" "${linux_tags[@]}" "$@"
# Windows next: reuse that frontend (-s) and keep the Linux binary (no -clean).
go run ./tools/release wails build -s -platform windows/amd64 -o quartz.exe "${flags[@]}" "$@"
