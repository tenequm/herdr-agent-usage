#!/usr/bin/env bash
# Setup action entry. The binary normally comes from the source-only manifest
# build hook. This fallback remains for installations that predate that hook.
# Resolution lives in a user-initiated, latency-tolerant path, never in
# run-usagebar.sh, which is also the hot path for concurrent event handlers.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$SCRIPT_DIR/ensure-binary.sh"

exec "$SCRIPT_DIR/run-usagebar.sh" setup "$@"
