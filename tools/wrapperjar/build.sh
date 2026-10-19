#!/bin/sh
# Rebuilds internal/wrapperjar/wrapper.jar from src/ for Java 8 and newer.
# Needs a JDK 17+ (javac --release 8, jar --date). The output is
# reproducible: the same source always gives the same bytes.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out="$here/../../internal/wrapperjar/wrapper.jar"
jdk=${JAVA_HOME:?set JAVA_HOME to a JDK 17 or newer}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
"$jdk/bin/javac" --release 8 -Xlint:-options -encoding UTF-8 -d "$tmp/classes" "$here/src/wrapper/Main.java"
"$jdk/bin/jar" --create --file "$tmp/wrapper.jar" --date=2000-01-01T00:00:00Z --no-manifest -C "$tmp/classes" .
mv "$tmp/wrapper.jar" "$out"
sha256sum "$out"
