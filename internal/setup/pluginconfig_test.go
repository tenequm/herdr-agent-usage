/**
 * Tests for plugin config seed/parse
 */
package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/senna-lang/herdr-agent-usage/internal/core"
)

func TestParsePluginConfigTOML_Defaults(t *testing.T) {
	raw := DefaultPluginConfigTOML(DefaultPluginConfig)
	if !contains(raw, "[ui]") || !contains(raw, `limit_percent = "remaining"`) {
		t.Fatalf("seed missing remaining default:\n%s", raw)
	}
	cfg := ParsePluginConfigTOML(raw)
	if !cfg.NotifyEnabled {
		t.Fatal("expected enabled")
	}
	if cfg.LimitPercent != core.LimitPercentRemaining {
		t.Fatalf("limit_percent=%q", cfg.LimitPercent)
	}
	if !reflect.DeepEqual(cfg.RemainingThresholds, []int{50, 20, 10, 5}) {
		t.Fatalf("thresholds=%v", cfg.RemainingThresholds)
	}
}

func TestParsePluginConfigTOML_CacheDisplay(t *testing.T) {
	if !DefaultPluginConfig.CacheDisplay {
		t.Fatal("cache display must default to enabled")
	}
	raw := DefaultPluginConfigTOML(DefaultPluginConfig)
	if !contains(raw, "cache_display = true") {
		t.Fatalf("seed missing cache display default:\n%s", raw)
	}
	if ParsePluginConfigTOML("[ui]\ncache_display = false").CacheDisplay {
		t.Fatal("cache display setting ignored")
	}
}

func TestParsePluginConfigTOML_Sidebar(t *testing.T) {
	if !DefaultPluginConfig.Sidebar || !ParsePluginConfigTOML("").Sidebar {
		t.Fatal("sidebar must default to enabled")
	}
	if ParsePluginConfigTOML("[ui]\nsidebar = false").Sidebar {
		t.Fatal("sidebar=false was ignored")
	}
	if !contains(DefaultPluginConfigTOML(DefaultPluginConfig), "# sidebar = false") {
		t.Fatal("seed config does not document pane-only sidebar switch")
	}
}

func TestParsePluginConfigTOML_AccountEmail(t *testing.T) {
	if DefaultPluginConfig.AccountEmail || ParsePluginConfigTOML("").AccountEmail {
		t.Fatal("account_email must default to disabled")
	}
	if !ParsePluginConfigTOML("[ui]\naccount_email = true").AccountEmail {
		t.Fatal("account_email=true was ignored")
	}
	if !contains(DefaultPluginConfigTOML(DefaultPluginConfig), "account_email = false") {
		t.Fatal("seed config does not document account email switch")
	}
}

func TestParsePluginConfigTOML_AutoCheck(t *testing.T) {
	if !DefaultPluginConfig.AutoCheck || !ParsePluginConfigTOML("").AutoCheck {
		t.Fatal("automatic update checks must default to enabled")
	}
	if ParsePluginConfigTOML("[update]\nauto_check = false").AutoCheck {
		t.Fatal("auto_check=false was ignored")
	}
	if !contains(DefaultPluginConfigTOML(DefaultPluginConfig), "# auto_check = false") {
		t.Fatal("seed config does not document auto update switch")
	}
}

func TestParsePluginConfigTOML_StateDirAndUnsafeProfileIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := ParsePluginConfigTOML(`
[state]
dir = "~/.local/state/usagebar"

[[claude.profiles]]
id = "../escape"
config_dir = "/profiles/escape"

[[claude.profiles]]
id = ".hidden"
config_dir = "/profiles/hidden"

[[claude.profiles]]
id = "safe"
config_dir = "/profiles/safe"
`)
	if cfg.StateDir != filepath.Join(home, ".local", "state", "usagebar") {
		t.Fatalf("state dir = %q", cfg.StateDir)
	}
	if len(cfg.ClaudeProfiles) != 1 || cfg.ClaudeProfiles[0].ID != "safe" {
		t.Fatalf("profiles = %+v", cfg.ClaudeProfiles)
	}
	if !reflect.DeepEqual(cfg.InvalidProfileIDs, []string{"claude:../escape", "claude:.hidden"}) {
		t.Fatalf("invalid ids = %v", cfg.InvalidProfileIDs)
	}
}

func TestParsePluginConfigTOML_LegacyProfileIDsWithoutStateDir(t *testing.T) {
	cfg := ParsePluginConfigTOML(`
[[claude.profiles]]
id = "../claude"
config_dir = "/profiles/claude"

[[codex.profiles]]
id = ".codex"
codex_home = "/profiles/codex"

[[grok.profiles]]
id = "grok/profile"
grok_home = "/profiles/grok"

[[opencode.profiles]]
id = "opencode\\profile"
data_dir = "/profiles/opencode"
`)

	if got := cfg.InvalidProfileIDs; len(got) != 0 {
		t.Fatalf("invalid ids = %v, want none without configured state dir", got)
	}
	if len(cfg.ClaudeProfiles) != 1 || cfg.ClaudeProfiles[0].ID != "../claude" {
		t.Fatalf("claude profiles = %+v", cfg.ClaudeProfiles)
	}
	if len(cfg.CodexProfiles) != 1 || cfg.CodexProfiles[0].ID != ".codex" {
		t.Fatalf("codex profiles = %+v", cfg.CodexProfiles)
	}
	if len(cfg.GrokProfiles) != 1 || cfg.GrokProfiles[0].ID != "grok/profile" {
		t.Fatalf("grok profiles = %+v", cfg.GrokProfiles)
	}
	if len(cfg.OpenCodeProfiles) != 1 || cfg.OpenCodeProfiles[0].ID != `opencode\profile` {
		t.Fatalf("opencode profiles = %+v", cfg.OpenCodeProfiles)
	}
}

func TestParsePluginConfigTOML_ProviderAllowlist(t *testing.T) {
	cfg := ParsePluginConfigTOML("[providers]\nenabled = [\"Claude\", \"unknown\", \"CODEX\", \"claude\"]")
	if !cfg.ProviderAllowlistConfigured {
		t.Fatal("non-empty provider allowlist was not marked configured")
	}
	if !reflect.DeepEqual(cfg.EnabledProviderFamilies, []string{"claude", "codex"}) {
		t.Fatalf("enabled providers = %v", cfg.EnabledProviderFamilies)
	}
	if !reflect.DeepEqual(cfg.UnknownProviderFamilies, []string{"unknown"}) {
		t.Fatalf("unknown providers = %v", cfg.UnknownProviderFamilies)
	}
	absent := ParsePluginConfigTOML("")
	if absent.ProviderAllowlistConfigured || len(absent.EnabledProviderFamilies) != 0 {
		t.Fatalf("absent allowlist changed default: %+v", absent)
	}
}

func TestParsePluginConfigTOML_InvalidProviderAllowlistFailsClosed(t *testing.T) {
	cfg := ParsePluginConfigTOML("[providers]\nenabled = [\"Claudee\", \"omp\", \"cursor\"]")
	if !cfg.ProviderAllowlistConfigured {
		t.Fatal("invalid non-empty provider allowlist was treated as absent")
	}
	if len(cfg.EnabledProviderFamilies) != 0 {
		t.Fatalf("enabled providers = %v", cfg.EnabledProviderFamilies)
	}
	if !reflect.DeepEqual(cfg.UnknownProviderFamilies, []string{"claudee", "omp", "cursor"}) {
		t.Fatalf("unknown providers = %v", cfg.UnknownProviderFamilies)
	}
}

func TestParsePluginConfigTOML_Custom(t *testing.T) {
	cfg := ParsePluginConfigTOML(`
[notify]
enabled = false
remaining_thresholds = [30, 10]
`)
	if cfg.NotifyEnabled {
		t.Fatal("expected disabled")
	}
	if cfg.LimitPercent != core.LimitPercentRemaining {
		t.Fatal("missing [ui] must stay remaining")
	}
	if !reflect.DeepEqual(cfg.RemainingThresholds, []int{30, 10}) {
		t.Fatalf("thresholds=%v", cfg.RemainingThresholds)
	}
}

func TestParsePluginConfigTOML_LimitPercentUsed(t *testing.T) {
	cfg := ParsePluginConfigTOML(`
[ui]
limit_percent = "used"
`)
	if cfg.LimitPercent != core.LimitPercentUsed {
		t.Fatalf("got %q", cfg.LimitPercent)
	}
}

func TestParsePluginConfigTOML_LimitPercentInvalidFallsBack(t *testing.T) {
	cfg := ParsePluginConfigTOML(`
[ui]
limit_percent = "burned"
`)
	if cfg.LimitPercent != core.LimitPercentRemaining {
		t.Fatalf("got %q", cfg.LimitPercent)
	}
}

func TestParsePluginConfigTOML_ClaudeProfiles(t *testing.T) {
	cfg := ParsePluginConfigTOML(`
[[claude.profiles]]
id = "claude"
label = "Claude"
config_dir = "/home/u/.claude"
claude_json_path = "/home/u/.claude.json"

[[claude.profiles]]
id = "claude-secondary"
label = "Claude (secondary)"
config_dir = "/home/u/.claude-m"
`)
	if len(cfg.ClaudeProfiles) != 2 {
		t.Fatalf("want 2 profiles, got %d", len(cfg.ClaudeProfiles))
	}
	p0 := cfg.ClaudeProfiles[0]
	if p0.ID != "claude" || p0.ConfigDir != "/home/u/.claude" || p0.JSONPath != "/home/u/.claude.json" {
		t.Fatalf("p0 = %+v", p0)
	}
	p1 := cfg.ClaudeProfiles[1]
	if p1.ID != "claude-secondary" || p1.Label != "Claude (secondary)" || p1.ConfigDir != "/home/u/.claude-m" {
		t.Fatalf("p1 = %+v", p1)
	}
	// notify defaults still apply when [notify] is absent.
	if !cfg.NotifyEnabled {
		t.Fatal("expected notify default enabled")
	}
}

func TestParsePluginConfigTOML_CodexProfiles(t *testing.T) {
	cfg := ParsePluginConfigTOML(`
[[codex.profiles]]
id = "codex"
label = "personal"
codex_home = "/home/u/.codex"

[[codex.profiles]]
id = "dev"
label = "product"
codex_home = "/home/u/.codex-dev"
`)
	if len(cfg.CodexProfiles) != 2 {
		t.Fatalf("want 2 profiles, got %d", len(cfg.CodexProfiles))
	}
	p0 := cfg.CodexProfiles[0]
	if p0.ID != "codex" || p0.Label != "personal" || p0.CodexHome != "/home/u/.codex" {
		t.Fatalf("p0 = %+v", p0)
	}
	p1 := cfg.CodexProfiles[1]
	if p1.ID != "dev" || p1.Label != "product" || p1.CodexHome != "/home/u/.codex-dev" {
		t.Fatalf("p1 = %+v", p1)
	}
}

func TestParsePluginConfigTOML_GrokAndOpenCodeProfiles(t *testing.T) {
	cfg := ParsePluginConfigTOML(`
[[grok.profiles]]
id = "grok-personal"
label = "Grok Personal"
grok_home = "/home/u/.grok-personal"

[[opencode.profiles]]
id = "opencode-work"
label = "OpenCode Work"
data_dir = "/home/u/.local/share/opencode-work"
`)

	if len(cfg.GrokProfiles) != 1 {
		t.Fatalf("Grok profiles = %+v", cfg.GrokProfiles)
	}
	if got := cfg.GrokProfiles[0]; got.ID != "grok-personal" || got.Label != "Grok Personal" || got.GrokHome != "/home/u/.grok-personal" {
		t.Fatalf("Grok profile = %+v", got)
	}
	if len(cfg.OpenCodeProfiles) != 1 {
		t.Fatalf("OpenCode profiles = %+v", cfg.OpenCodeProfiles)
	}
	if got := cfg.OpenCodeProfiles[0]; got.ID != "opencode-work" || got.Label != "OpenCode Work" || got.DataDir != "/home/u/.local/share/opencode-work" {
		t.Fatalf("OpenCode profile = %+v", got)
	}
}

func TestParsePluginConfigTOML_NoProfilesByDefault(t *testing.T) {
	cfg := ParsePluginConfigTOML(DefaultPluginConfigTOML(DefaultPluginConfig))
	if len(cfg.ClaudeProfiles) != 0 {
		t.Fatalf("seed must not define active Claude profiles, got %+v", cfg.ClaudeProfiles)
	}
	if len(cfg.CodexProfiles) != 0 {
		t.Fatalf("seed must not define active Codex profiles, got %+v", cfg.CodexProfiles)
	}
}

func TestSeedPluginConfigIfMissing(t *testing.T) {
	dir := t.TempDir()
	if !SeedPluginConfigIfMissing(dir) {
		t.Fatal("first seed should write")
	}
	if SeedPluginConfigIfMissing(dir) {
		t.Fatal("second seed should not write")
	}
	text, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(text), "[notify]") {
		t.Fatal("missing [notify]")
	}
	if !LoadPluginConfig(dir).NotifyEnabled {
		t.Fatal("expected enabled")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

func TestParsePluginConfigTOML_TypeMismatchFailsClosed(t *testing.T) {
	cfg := ParsePluginConfigTOML("[providers]\nenabled = \"claude\"\n[ui]\nsidebar = \"false\"\n")
	if cfg.DecodeError == "" || !cfg.ProviderAllowlistConfigured || len(cfg.EnabledProviderFamilies) != 0 || cfg.Sidebar {
		t.Fatalf("config did not fail closed: %+v", cfg)
	}
}

func TestParsePluginConfigTOML_StateRootAndClaudeIDSafety(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, root := range []string{"/", home, filepath.Dir(home)} {
		cfg := ParsePluginConfigTOML("[state]\ndir = \"" + root + "\"\n")
		if cfg.StateDir != "" || cfg.InvalidStateDir == "" {
			t.Fatalf("unsafe root %q accepted: %+v", root, cfg)
		}
	}
	raw := "[state]\ndir = \"" + filepath.Join(home, "state") + "\"\n" +
		"[[claude.profiles]]\nid = \"work\"\nconfig_dir = \"/tmp/a\"\n" +
		"[[claude.profiles]]\nid = \"Work\"\nconfig_dir = \"/tmp/b\"\n" +
		"[[codex.profiles]]\nid = \".work\"\ncodex_home = \"/tmp/c\"\n"
	cfg := ParsePluginConfigTOML(raw)
	if len(cfg.ClaudeProfiles) != 1 || len(cfg.InvalidProfileIDs) != 1 || len(cfg.CodexProfiles) != 1 {
		t.Fatalf("profile validation = %+v", cfg)
	}
}
