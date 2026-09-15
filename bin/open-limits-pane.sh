#!/usr/bin/env bash
# Action: open the limits plugin pane as a focused overlay.
set -euo pipefail
HERDR_BIN="${HERDR_BIN_PATH:-herdr}"
exec "$HERDR_BIN" plugin pane open \
  --plugin usagebar \
  --entrypoint limits \
  --placement overlay \
  --focus
