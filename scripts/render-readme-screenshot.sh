#!/usr/bin/env bash
# Render docs/assets/agent-usage-pane.png with VHS from a made-up fixture HOME
# (Claude x2, Codex, OpenCode Go, Grok via OMP, DeepSeek API) and a fake herdr.
# Homebrew vhs 0.12.0 writes no output (charmbracelet/vhs#787); run with
# VHS=/path/to/vhs-0.11.0 scripts/render-readme-screenshot.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
vhs_bin="${VHS:-vhs}"
out="$root/docs/assets/agent-usage-pane.png"

work="$(mktemp -d "${TMPDIR:-/tmp}/herdr-readme-render.XXXXXX")"
trap 'rm -rf "$work"' EXIT

write_fixture() {
  local H="$1"
  local now ms day iso week_start mid_week
  now=$(date +%s); ms=$((now*1000)); day=$(date -u +%Y/%m/%d); iso=$(date -u +%Y-%m-%dT%H-%M-%S)
  mkdir -p "$H/.config/usagebar" "$H/.claude" "$H/.claude-work" "$H/.codex/sessions/$day" \
    "$H/state/claude/personal" "$H/state/claude/work" "$H/.local/share/opencode" "$H/.cache/opencode" \
    "$H/.omp/agent" "$H/bin" "$H/work/api" "$H/work/docs" "$H/work/old" "$H/work/scratch"

  # No [providers].enabled: an allowlist makes collectors skip borrowed
  # windows, and Grok's only file-based source here is OMP's usage_history.
  cat >"$H/.config/usagebar/config.toml" <<TOML
[ui]
sidebar = false
cache_display = false
account_email = false
[update]
auto_check = false
[state]
dir = "$H/state"
[[claude.profiles]]
id = "personal"
label = "personal"
config_dir = "~/.claude"
[[claude.profiles]]
id = "work"
label = "work"
config_dir = "~/.claude-work"
TOML

  # Claude: statusLine caches plus a 15-minute burn on work's 5h window.
  echo '{"oauthAccount":{"organizationType":"claude_max"}}' >"$H/.claude.json"
  echo '{"oauthAccount":{"organizationType":"claude_team"}}' >"$H/.claude-work/.claude.json"
  claude_cache() {
    printf '{"fiveHour":{"usedPercentage":%s,"resetsAt":%s,"windowMinutes":300},"sevenDay":{"usedPercentage":%s,"resetsAt":%s,"windowMinutes":10080},"fetchedAtMs":%s}\n' \
      "$2" $((now+$3)) "$4" $((now+$5)) "$ms" >"$H/state/claude/$1/claude-limits-latest.json"
  }
  claude_cache personal 22 11220 18 480000
  claude_cache work 58 5400 44 245000
  printf '{"work:primary":[{"T":%s,"Used":46},{"T":%s,"Used":50},{"T":%s,"Used":54}]}\n' \
    $((ms-900000)) $((ms-600000)) $((ms-300000)) >"$H/state/usage-history.json"

  # Codex: one fresh rollout with rate_limits.
  printf '{"timestamp":"%s","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":37,"window_minutes":300,"resets_at":%s},"secondary":{"used_percent":72,"window_minutes":10080,"resets_at":%s},"plan_type":"plus"}}}\n' \
    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" $((now+8100)) $((now+300000)) >"$H/.codex/sessions/$day/rollout-$iso-demo.jsonl"

  # OpenCode: opencode-go (subscription) and deepseek (API) rows. A fresh
  # failed-fetch web cache entry keeps the collector off the network and cookies.
  printf '{"fetchedAtMs":%s,"outcome":"fetch-failed"}\n' "$ms" >"$H/state/opencode-go-web.json"
  cat >"$H/.cache/opencode/models.json" <<'JSON'
{"deepseek":{"name":"DeepSeek","models":{"deepseek-chat":{"limit":{"context":128000}},"deepseek-reasoner":{"limit":{"context":128000}}}},
 "opencode-go":{"name":"OpenCode Go","models":{"kimi-k2":{"limit":{"context":262144}}}}}
JSON
  week_start=$(( now - now % 86400 - ($(date -u +%u) - 1) * 86400 ))
  mid_week=$(( (week_start + now - 5*3600) / 2 ))
  # Columns: epoch session provider model total_tokens cost
  {
    local o
    for o in 10800:ses_go0 9000:ses_go1 7200:ses_go1 5400:ses_go1 3000:ses_go1 600:ses_go1; do
      echo "$((now-${o%%:*})) ${o##*:} opencode-go kimi-k2 190000 0.70"
    done
    echo "$mid_week ses_go0 opencode-go kimi-k2 800000 2.10"
    echo "$((mid_week+3600)) ses_go0 opencode-go glm-4.6 760000 2.10"
    echo "$((now-20*86400)) ses_go0 opencode-go kimi-k2 2400000 6.20"
    echo "$((now-16*86400)) ses_go0 opencode-go glm-4.6 2900000 7.10"
    echo "$((now-12*86400)) ses_go0 opencode-go kimi-k2 2600000 6.30"
    echo "$((week_start-86400)) ses_go0 opencode-go kimi-k2 1900000 5.00"
    echo "$((now-1200)) ses_ds1 deepseek deepseek-chat 609000 0.19"
    echo "$((now-3600)) ses_ds1 deepseek deepseek-reasoner 322000 0.21"
    echo "$((now-10800)) ses_ds1 deepseek deepseek-chat 426000 0.08"
    echo "$((now-32400)) ses_ds0 deepseek deepseek-chat 538000 0.10"
    echo "$((now-50400)) ses_ds0 deepseek deepseek-reasoner 165000 0.12"
    echo "$((now-2*86400)) ses_ds0 deepseek deepseek-chat 2400000 0.80"
    echo "$((now-3*86400)) ses_ds0 deepseek deepseek-reasoner 1900000 0.55"
    echo "$((now-5*86400)) ses_ds0 deepseek deepseek-chat 3100000 0.90"
    echo "$((now-9*86400)) ses_ds0 deepseek deepseek-chat 6000000 1.90"
    echo "$((now-15*86400)) ses_ds0 deepseek deepseek-reasoner 8000000 2.40"
    echo "$((now-24*86400)) ses_ds0 deepseek deepseek-chat 7500000 2.15"
  } | awk -v H="$H" -v ms="$ms" '
  BEGIN {
    print "CREATE TABLE session (id TEXT PRIMARY KEY, directory TEXT, time_created INTEGER, time_updated INTEGER, time_archived INTEGER);"
    print "CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT, time_created INTEGER, time_updated INTEGER, data TEXT);"
    printf "INSERT INTO session VALUES (\"ses_go1\",\"%s/work/api\",%d,%d,NULL),(\"ses_go0\",\"%s/work/old\",%d,%d,NULL),(\"ses_ds1\",\"%s/work/docs\",%d,%d,NULL),(\"ses_ds0\",\"%s/work/scratch\",%d,%d,NULL);\n", H, ms-86400000, ms, H, ms-2592000000, ms-3600000, H, ms-86400000, ms, H, ms-2592000000, ms-7200000
  }
  {
    t = $1 * 1000; tot = $5
    in_ = int(tot * 0.30); out = int(tot * 0.02); cr = tot - in_ - out
    data = sprintf("{\"role\":\"assistant\",\"providerID\":\"%s\",\"modelID\":\"%s\",\"cost\":%s,\"tokens\":{\"input\":%d,\"output\":%d,\"reasoning\":0,\"cache\":{\"read\":%d,\"write\":0}},\"time\":{\"created\":%d,\"completed\":%d}}", $3, $4, $6, in_, out, cr, t, t + 20000)
    printf "INSERT INTO message VALUES (\"msg_%03d\",\"%s\",%d,%d,'"'"'%s'"'"');\n", NR, $2, t, t + 20000, data
  }' | sqlite3 "$H/.local/share/opencode/opencode.db"

  # Grok: no ~/.grok, so the collector borrows OMP's observation of the xAI account.
  sqlite3 "$H/.omp/agent/agent.db" <<SQL
CREATE TABLE usage_history (provider TEXT, account_key TEXT, email TEXT, account_id TEXT, limit_id TEXT, label TEXT, window_label TEXT, used_fraction REAL, status TEXT, resets_at INTEGER, recorded_at INTEGER);
INSERT INTO usage_history VALUES
 ('xai-oauth','oauth|email:dev@example.com','dev@example.com','','xai-oauth:credits:1w','Credits','7d',0.88,'ok',$((ms+16*3600000)),$ms),
 ('xai-oauth','oauth|email:dev@example.com','dev@example.com','','xai-oauth:included:1mo','Included','30d',0.34,'ok',$((ms+19*86400000)),$ms);
SQL

  # Fake herdr: two open OpenCode panes, one per backend, so the DeepSeek block
  # and the pane-share rows have something to attach to.
  cat >"$H/bin/herdr" <<SH
#!/bin/sh
if [ "\$1 \$2" = "pane list" ]; then
  printf '%s\n' '{"result":{"panes":[{"pane_id":"p1","agent":"opencode","label":"api-server","cwd":"$H/work/api","agent_session":{"kind":"opencode","value":"ses_go1"}},{"pane_id":"p2","agent":"opencode","label":"docs-site","cwd":"$H/work/docs","agent_session":{"kind":"opencode","value":"ses_ds1"}}]}}'
else
  printf '%s\n' '{"result":{}}'
fi
SH
  chmod +x "$H/bin/herdr"
}

(cd "$root" && CGO_ENABLED=0 go build -o "$work/usagebar" ./cmd/usagebar)
home="$work/home"
write_fixture "$home"

# 1514x1630 gives the 70x38 grid; under 38 rows the pane drops to its compact layout.
# The trailing Sleep matters: a tape that ends on Screenshot writes nothing.
cat >"$work/pane.tape" <<TAPE
Output "$work/pane.mp4"
Set Shell zsh
Set FontFamily "Monaco"
Set FontSize 32
Set Width 1514
Set Height 1630
Set Padding 40
Set Margin 0
Set BorderRadius 0
Set Theme { "background": "#0b1424", "foreground": "#c0caf5", "cursor": "#0b1424", "selection": "#1e2a44", "black": "#1e2a44", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#c0caf5", "brightBlack": "#545c7e", "brightRed": "#ff899d", "brightGreen": "#9fe044", "brightYellow": "#faba4a", "brightBlue": "#8db0ff", "brightMagenta": "#c7a9ff", "brightCyan": "#a4daff", "brightWhite": "#e6ebff" }
Set CursorBlink false

Hide
Type \`unset HERDR_ENV HERDR_SOCKET HERDR_SOCKET_PATH CLAUDE_CONFIG_DIR CODEX_HOME GROK_HOME GROK_BIN XDG_DATA_HOME XDG_CACHE_HOME XDG_CONFIG_HOME XDG_STATE_HOME OPENCODE_DB OPENCODE_DATA_DIR OPENCODE_GO_COOKIE OPENCODE_MODELS_PATH \${(k)parameters[(I)USAGEBAR_*]}; export HOME=$home HERDR_PLUGIN_CONFIG_DIR=$home/.config/usagebar HERDR_BIN_PATH=$home/bin/herdr USAGEBAR_DISABLE_BROWSER_COOKIES=1 PATH=/usr/bin:/bin; clear; exec $work/usagebar limits --all\` Enter
Sleep 3s
Show
Sleep 1s
Screenshot "$work/pane.png"
Sleep 500ms
TAPE

(cd "$work" && "$vhs_bin" pane.tape)
rm -f "$work/pane.mp4"
[[ -s "$work/pane.png" ]] || { echo "vhs wrote no screenshot (vhs 0.12.0? see header)" >&2; exit 1; }
mv "$work/pane.png" "$out"
echo "wrote $out"
