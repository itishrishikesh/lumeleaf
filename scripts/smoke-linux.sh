#!/usr/bin/env bash
set -euo pipefail
runtime_dir=$(mktemp -d /tmp/lumeleaf-weston.XXXXXX)
chmod 700 "$runtime_dir"
XDG_RUNTIME_DIR="$runtime_dir" weston --backend=headless-backend.so --socket=lumeleaf-headless --idle-time=0 >/tmp/lumeleaf-weston.log 2>&1 &
weston_pid=$!
trap 'kill "$weston_pid" 2>/dev/null || true' EXIT
for _ in $(seq 1 100); do
  test -S "$runtime_dir/lumeleaf-headless" && break
  sleep 0.05
done
test -S "$runtime_dir/lumeleaf-headless"
XDG_RUNTIME_DIR="$runtime_dir" WAYLAND_DISPLAY=lumeleaf-headless go run -tags desktop ./cmd/lumeleaf --smoke-script testdata/smoke/basic.json --report /tmp/lumeleaf-smoke.json
test -s /tmp/lumeleaf-smoke.json
