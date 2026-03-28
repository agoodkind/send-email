#!/usr/bin/env bash
# Build and install send-email to /opt/scripts/send-email (requires write access).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$root"

if ! command -v go >/dev/null 2>&1; then
	echo "Error: go not found in PATH" >&2
	exit 1
fi

go build -o send-email .

target="/opt/scripts/send-email"
if [[ ! -w "$(dirname "$target")" ]]; then
	echo "Installing to $target (sudo)..."
	sudo install -m 0755 send-email "$target"
else
	install -m 0755 send-email "$target"
fi

echo "Installed $target"
