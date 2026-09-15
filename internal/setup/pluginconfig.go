/**
 * Seeds and loads the plugin-specific config (HERDR_PLUGIN_CONFIG_DIR).
 * Lives in a separate space from the main Herdr config.toml.
 *
 * Parsing uses a real TOML decoder (BurntSushi) so array-of-tables
 * ([[claude.profiles]], [[codex.profiles]]) is handled correctly; the seed body
 * is still authored as a string for readable inline comments.
 */
package setup

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/senna-lang/herdr-agent-usage/internal/core"
	"github.com/senna-lang/herdr-agent-usage/internal/pathutil"
	"github.com/senna-lang/herdr-agent-usage/internal/pluginstate"
	"github.com/senna-lang/herdr-agent-usage/internal/providers"
	"github.com/senna-lang/herdr-agent-usage/internal/providers/claude"
	"github.com/senna-lang/herdr-agent-usage/internal/providers/codex"
	"github.com/senna-lang/herdr-agent-usage/internal/providers/grok"
	"github.com/senna-lang/herdr-agent-usage/internal/providers/opencode"
)

// DefaultRemainingThresholds are the toast remaining-% buckets.
var DefaultRemainingThresholds = []int{50, 20, 10, 5}

// PluginConfig is the plugin-local config shape.
type PluginConfig struct {
	RemainingThresholds []int
	// NotifyEnabled is the plugin-side intent (separate from host toast delivery).
	NotifyEnabled bool
	// LimitPercent is the presentation direction for quota percentages.
	// remaining (default) shows headroom; used shows consumption. Notify
	// firing still uses remaining thresholds.
	LimitPercent core.LimitPercent
	// CacheDisplay controls both sidebar cache tokens and the Agent Usage pane's
	// red-band cache warning.
	CacheDisplay bool
	// AccountEmail allows the limits overlay to read and display local account
	// emails. It defaults off so upstream rendering remains unchanged.
	AccountEmail bool
	// Sidebar controls all metadata publishing and the idle watcher.
	Sidebar bool
	// AutoCheck controls quiet event-triggered update checks. Explicit checks
	// remain available regardless of this value.
	AutoCheck bool
	// StateDir is the expanded, absolute plugin-owned runtime state root.
	StateDir string
	// InvalidStateDir is retained only for setup diagnostics.
	InvalidStateDir string
	// DecodeError is shown by setup; security-sensitive tables fail closed.
	DecodeError string
	// InvalidProfileIDs are rejected path-unsafe ids retained for setup warnings.
	InvalidProfileIDs []string
	// ProviderAllowlistConfigured distinguishes an absent or explicitly empty
	// list from a non-empty list whose entries are all invalid. The latter must
	// fail closed rather than silently enabling every provider.
	ProviderAllowlistConfigured bool
	// EnabledProviderFamilies bounds all quota collection when the allowlist is
	// configured. It may be empty when every configured entry was invalid.
	// Values are canonical provider family ids from providers.Registrations.
	EnabledProviderFamilies []string
	// UnknownProviderFamilies retains ignored ids for setup diagnostics only.
	UnknownProviderFamilies []string
	// ClaudeProfiles are the configured [[claude.profiles]] entries (unresolved).
	// Empty means the single implicit "claude" profile is synthesized downstream.
	ClaudeProfiles []claude.ProfileSpec
	// CodexProfiles are the configured [[codex.profiles]] entries (unresolved).
	// Empty means the single implicit "codex" profile is synthesized downstream.
	CodexProfiles    []codex.ProfileSpec
	GrokProfiles     []grok.ProfileSpec
	OpenCodeProfiles []opencode.ProfileSpec
}

// DefaultPluginConfig is the seed default.
var DefaultPluginConfig = PluginConfig{
	RemainingThresholds: append([]int(nil), DefaultRemainingThresholds...),
	NotifyEnabled:       true,
	LimitPercent:        core.LimitPercentRemaining,
	CacheDisplay:        true,
	AccountEmail:        false,
	Sidebar:             true,
	AutoCheck:           true,
}

// pluginConfigWire mirrors the on-disk TOML shape for decoding.
type pluginConfigWire struct {
	Notify struct {
		Enabled             *bool `toml:"enabled"`
		RemainingThresholds []int `toml:"remaining_thresholds"`
	} `toml:"notify"`
	UI struct {
		LimitPercent *string `toml:"limit_percent"`
		CacheDisplay *bool   `toml:"cache_display"`
		AccountEmail *bool   `toml:"account_email"`
		Sidebar      *bool   `toml:"sidebar"`
	} `toml:"ui"`
	Providers struct {
		Enabled []string `toml:"enabled"`
	} `toml:"providers"`
	Update struct {
		AutoCheck *bool `toml:"auto_check"`
	} `toml:"update"`
	State struct {
		Dir string `toml:"dir"`
	} `toml:"state"`
	Claude struct {
		Profiles []profileWire `toml:"profiles"`
	} `toml:"claude"`
	Codex struct {
		Profiles []codexProfileWire `toml:"profiles"`
	} `toml:"codex"`

	Grok struct {
		Profiles []grokProfileWire `toml:"profiles"`
	} `toml:"grok"`
	OpenCode struct {
		Profiles []openCodeProfileWire `toml:"profiles"`
	} `toml:"opencode"`
}

type profileWire struct {
	ID             string `toml:"id"`
	Label          string `toml:"label"`
	ConfigDir      string `toml:"config_dir"`
	ClaudeJSONPath string `toml:"claude_json_path"`
}

type codexProfileWire struct {
	ID        string `toml:"id"`
	Label     string `toml:"label"`
	CodexHome string `toml:"codex_home"`
}

type grokProfileWire struct {
	ID       string `toml:"id"`
	Label    string `toml:"label"`
	GrokHome string `toml:"grok_home"`
}

type openCodeProfileWire struct {
	ID      string `toml:"id"`
	Label   string `toml:"label"`
	DataDir string `toml:"data_dir"`
}

// ResolvePluginConfigDir resolves the config directory.
// HERDR_PLUGIN_CONFIG_DIR → else ~/.config/herdr/plugins/config/usagebar
func ResolvePluginConfigDir(env map[string]string) string {
	if fromEnv := env["HERDR_PLUGIN_CONFIG_DIR"]; fromEnv != "" {
		return fromEnv
	}
	if xdg := env["XDG_CONFIG_HOME"]; xdg != "" {
		return filepath.Join(xdg, "herdr", "plugins", "config", "usagebar")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herdr", "plugins", "config", "usagebar")
}

// PluginConfigPath is config.toml under the plugin config dir.
func PluginConfigPath(configDir string) string {
	return filepath.Join(configDir, "config.toml")
}

// DefaultPluginConfigTOML is the default config.toml body.
func DefaultPluginConfigTOML(config PluginConfig) string {
	if len(config.RemainingThresholds) == 0 {
		config = DefaultPluginConfig
	}
	parts := make([]string, len(config.RemainingThresholds))
	for i, n := range config.RemainingThresholds {
		parts[i] = strconv.Itoa(n)
	}
	thresholds := strings.Join(parts, ", ")
	enabled := "false"
	if config.NotifyEnabled {
		enabled = "true"
	}
	return strings.Join([]string{
		"# Agent Usage (usagebar) plugin config",
		"# Path: herdr plugin config-dir usagebar",
		"",
		"[notify]",
		"enabled = " + enabled,
		"# remaining % thresholds that may fire a toast (once per window/bucket)",
		"remaining_thresholds = [" + thresholds + "]",
		"",
		"[ui]",
		"# remaining (default): % left / higher is safer. used: fill as you burn.",
		`limit_percent = "` + string(core.ParseLimitPercent(string(config.LimitPercent))) + `"`,
		"# Set false to hide cache data from both sidebar and Agent Usage.",
		"cache_display = " + strconv.FormatBool(config.CacheDisplay),
		"# Opt in to showing locally read Claude and Codex account emails.",
		"account_email = " + strconv.FormatBool(config.AccountEmail),
		"# Set false for a pane-only installation with no agent-pane metadata.",
		"# sidebar = false",
		"",
		"[providers]",
		"# Limit every collector and the Agent Usage pane to these provider families.",
		"# Empty keeps the default behavior (all supported provider families).",
		"# enabled = [\"claude\", \"codex\"]",
		"",
		"[update]",
		"# Set false to disable event-triggered network checks; the action still works.",
		"# auto_check = false",
		"",
		"[state]",
		"# Put every runtime file under one private plugin-owned root.",
		"# dir = \"~/.local/state/herdr-agent-usage\"",
		"",
		"# Multi-account Claude: uncomment and add one block per account.",

		"# Absence of any profile keeps the single default account (fully backward",
		"# compatible). config_dir must be unique per profile and may use ~.",
		"#",
		"# Once any profile exists, declare the default account too: bare `claude`",
		"# sets no CLAUDE_CONFIG_DIR, so the ~/.claude account is only recorded if",
		"# a profile claims that dir. `usagebar setup` warns when it is uncovered.",
		"#",
		"# [[claude.profiles]]",
		"# id = \"claude\"",
		"# label = \"Claude\"",
		"# config_dir = \"~/.claude\"",
		"# claude_json_path = \"~/.claude.json\"   # optional; defaults per config_dir",
		"#",
		"# [[claude.profiles]]",
		"# id = \"claude-secondary\"",
		"# label = \"Claude (secondary)\"",
		"# config_dir = \"~/.claude-secondary\"",
		"",
		"# Multi-account Codex: uncomment and add one block per CODEX_HOME.",
		"# Absence of any profile keeps the single default account at ~/.codex.",
		"# Once any profile exists, declare the default account too: bare `codex`",
		"# sets no CODEX_HOME, so ~/.codex is only recorded if a profile claims",
		"# that dir. `usagebar setup` warns when it is uncovered.",
		"#",
		"# [[codex.profiles]]",
		"# id = \"codex\"",
		"# label = \"personal\"",
		"# codex_home = \"~/.codex\"",
		"#",
		"# [[codex.profiles]]",
		"# id = \"dev\"",
		"# label = \"product\"",
		"# codex_home = \"~/.codex-dev\"",
		"",
		"# Multi-account Grok: one GROK_HOME per profile.",
		"# [[grok.profiles]]",
		"# id = \"grok\"",
		"# label = \"personal\"",
		"# grok_home = \"~/.grok\"",
		"#",
		"# Multi-account OpenCode: one OPENCODE_DATA_DIR per profile.",
		"# [[opencode.profiles]]",
		"# id = \"opencode\"",
		"# label = \"personal\"",
		"# data_dir = \"~/.local/share/opencode\"",
		"",
	}, "\n")
}

// validThresholds keeps the historical rule: every value must be 1..100, else
// the whole set is rejected in favor of the default.
func validThresholds(in []int) ([]int, bool) {
	if len(in) == 0 {
		return nil, false
	}
	out := make([]int, 0, len(in))
	for _, n := range in {
		if n <= 0 || n > 100 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

type configRecovery struct {
	NotifyDecoded, UIDecoded, ProvidersDecoded, UpdateDecoded bool
	NotifyEnabledPresent, UISidebarPresent                    bool
	ProvidersEnabledPresent, UpdateAutoCheckPresent           bool
}

func recoverPluginConfigWire(raw string) (pluginConfigWire, configRecovery) {
	var wire pluginConfigWire
	var tables map[string]toml.Primitive
	metadata, err := toml.Decode(raw, &tables)
	if err != nil {
		return wire, scanConfigKeyPresence(raw)
	}

	var decoded pluginConfigWire
	recovery := configRecovery{
		NotifyEnabledPresent:    metadata.IsDefined("notify", "enabled"),
		UISidebarPresent:        metadata.IsDefined("ui", "sidebar"),
		ProvidersEnabledPresent: metadata.IsDefined("providers", "enabled"),
		UpdateAutoCheckPresent:  metadata.IsDefined("update", "auto_check"),
	}
	if value, ok := tables["notify"]; ok {
		recovery.NotifyDecoded = metadata.PrimitiveDecode(value, &decoded.Notify) == nil
		if recovery.NotifyDecoded {
			wire.Notify = decoded.Notify
		}
	}
	if value, ok := tables["ui"]; ok {
		recovery.UIDecoded = metadata.PrimitiveDecode(value, &decoded.UI) == nil
		if recovery.UIDecoded {
			wire.UI = decoded.UI
		}
	}
	if value, ok := tables["providers"]; ok {
		recovery.ProvidersDecoded = metadata.PrimitiveDecode(value, &decoded.Providers) == nil
		if recovery.ProvidersDecoded {
			wire.Providers = decoded.Providers
		}
	}
	if value, ok := tables["update"]; ok {
		recovery.UpdateDecoded = metadata.PrimitiveDecode(value, &decoded.Update) == nil
		if recovery.UpdateDecoded {
			wire.Update = decoded.Update
		}
	}
	if value, ok := tables["state"]; ok && metadata.PrimitiveDecode(value, &decoded.State) == nil {
		wire.State = decoded.State
	}
	if value, ok := tables["claude"]; ok && metadata.PrimitiveDecode(value, &decoded.Claude) == nil {
		wire.Claude = decoded.Claude
	}
	if value, ok := tables["codex"]; ok && metadata.PrimitiveDecode(value, &decoded.Codex) == nil {
		wire.Codex = decoded.Codex
	}
	if value, ok := tables["grok"]; ok && metadata.PrimitiveDecode(value, &decoded.Grok) == nil {
		wire.Grok = decoded.Grok
	}
	if value, ok := tables["opencode"]; ok && metadata.PrimitiveDecode(value, &decoded.OpenCode) == nil {
		wire.OpenCode = decoded.OpenCode
	}
	return wire, recovery
}

// ParsePluginConfigTOML decodes the plugin config. A type error recovers each
// unaffected top-level table independently; explicit security-sensitive keys
// in an invalid table fail closed.
func ParsePluginConfigTOML(raw string) PluginConfig {
	raw = strings.TrimPrefix(raw, "\ufeff")
	cfg := PluginConfig{
		NotifyEnabled:       DefaultPluginConfig.NotifyEnabled,
		RemainingThresholds: append([]int(nil), DefaultPluginConfig.RemainingThresholds...),
		LimitPercent:        DefaultPluginConfig.LimitPercent,
		CacheDisplay:        DefaultPluginConfig.CacheDisplay,
		AccountEmail:        DefaultPluginConfig.AccountEmail,
		Sidebar:             DefaultPluginConfig.Sidebar,
		AutoCheck:           DefaultPluginConfig.AutoCheck,
	}

	var wire pluginConfigWire
	var recovery configRecovery
	if _, err := toml.Decode(raw, &wire); err != nil {
		cfg.DecodeError = err.Error()
		wire, recovery = recoverPluginConfigWire(raw)
	}
	if wire.Notify.Enabled != nil {
		cfg.NotifyEnabled = *wire.Notify.Enabled
	}
	if recovery.NotifyEnabledPresent && !recovery.NotifyDecoded {
		cfg.NotifyEnabled = false
	}
	if thr, ok := validThresholds(wire.Notify.RemainingThresholds); ok {
		cfg.RemainingThresholds = thr
	}
	if wire.UI.LimitPercent != nil {
		cfg.LimitPercent = core.ParseLimitPercent(*wire.UI.LimitPercent)
	}
	if wire.UI.CacheDisplay != nil {
		cfg.CacheDisplay = *wire.UI.CacheDisplay
	}
	if wire.UI.AccountEmail != nil {
		cfg.AccountEmail = *wire.UI.AccountEmail
	}
	if wire.UI.Sidebar != nil {
		cfg.Sidebar = *wire.UI.Sidebar
	}
	if recovery.UISidebarPresent && !recovery.UIDecoded {
		cfg.Sidebar = false
	}
	if wire.Update.AutoCheck != nil {
		cfg.AutoCheck = *wire.Update.AutoCheck
	}
	if recovery.UpdateAutoCheckPresent && !recovery.UpdateDecoded {
		cfg.AutoCheck = false
	}
	home, _ := os.UserHomeDir()
	if wire.State.Dir != "" {
		stateDir := pathutil.ExpandHome(wire.State.Dir, home)
		if filepath.IsAbs(stateDir) && validStateRoot(stateDir, home) {
			cfg.StateDir = stateDir
		} else {
			cfg.InvalidStateDir = wire.State.Dir
		}
	}

	quotaFamilies := make(map[string]bool)
	for _, id := range providers.IDsWithCapability(providers.CapOwnsSubscriptionQuota) {
		quotaFamilies[id] = true
	}
	seenFamilies := make(map[string]bool)
	cfg.ProviderAllowlistConfigured = len(wire.Providers.Enabled) > 0 ||
		(recovery.ProvidersEnabledPresent && !recovery.ProvidersDecoded)
	for _, rawID := range wire.Providers.Enabled {
		id := strings.ToLower(strings.TrimSpace(rawID))
		if id == "" || seenFamilies[id] {
			continue
		}
		seenFamilies[id] = true
		if quotaFamilies[id] {
			cfg.EnabledProviderFamilies = append(cfg.EnabledProviderFamilies, id)
		} else {
			cfg.UnknownProviderFamilies = append(cfg.UnknownProviderFamilies, id)
		}
	}

	seenClaudeIDs := make([]string, 0, len(wire.Claude.Profiles))
	for _, p := range wire.Claude.Profiles {
		duplicateID := false
		for _, seenID := range seenClaudeIDs {
			if strings.EqualFold(seenID, p.ID) {
				duplicateID = true
				break
			}
		}
		if cfg.StateDir != "" && duplicateID {
			cfg.InvalidProfileIDs = append(cfg.InvalidProfileIDs, "claude:"+p.ID)
			continue
		}
		if cfg.StateDir != "" && !validProfileID(p.ID) {
			cfg.InvalidProfileIDs = append(cfg.InvalidProfileIDs, "claude:"+p.ID)
			continue
		}
		cfg.ClaudeProfiles = append(cfg.ClaudeProfiles, claude.ProfileSpec{
			ID:        p.ID,
			Label:     p.Label,
			ConfigDir: p.ConfigDir,
			JSONPath:  p.ClaudeJSONPath,
		})
		seenClaudeIDs = append(seenClaudeIDs, p.ID)
	}
	for _, p := range wire.Codex.Profiles {
		cfg.CodexProfiles = append(cfg.CodexProfiles, codex.ProfileSpec{
			ID:        p.ID,
			Label:     p.Label,
			CodexHome: p.CodexHome,
		})
	}
	for _, p := range wire.Grok.Profiles {
		cfg.GrokProfiles = append(cfg.GrokProfiles, grok.ProfileSpec{ID: p.ID, Label: p.Label, GrokHome: p.GrokHome})
	}
	for _, p := range wire.OpenCode.Profiles {
		cfg.OpenCodeProfiles = append(cfg.OpenCodeProfiles, opencode.ProfileSpec{ID: p.ID, Label: p.Label, DataDir: p.DataDir})
	}
	return cfg
}

func scanConfigKeyPresence(raw string) configRecovery {
	return configRecovery{
		NotifyEnabledPresent:    hasTOMLKey(raw, "notify", "enabled"),
		UISidebarPresent:        hasTOMLKey(raw, "ui", "sidebar"),
		ProvidersEnabledPresent: hasTOMLKey(raw, "providers", "enabled"),
		UpdateAutoCheckPresent:  hasTOMLKey(raw, "update", "auto_check"),
	}
}

func hasTOMLKey(raw, table, key string) bool {
	section := ""
	raw = strings.TrimPrefix(raw, "\ufeff")
	for _, rawLine := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = tomlSectionName(line)
			continue
		}
		lhs, rhs, ok := splitTOMLAssignment(line)
		if !ok {
			continue
		}
		lhs = compactTOMLKey(lhs)
		if lhs == table+"."+key || section == table && lhs == key {
			return true
		}
		if lhs == table && inlineTOMLTableHasKey(rhs, key) {
			return true
		}
	}
	return false
}

func stripTOMLComment(line string) string {
	const (
		doubleQuote = byte(34)
		singleQuote = byte(39)
		escape      = byte(92)
		comment     = byte(35)
	)
	var quote byte
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if quote == doubleQuote && ch == escape && !escaped {
				escaped = true
				continue
			}
			if ch == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}
		if ch == doubleQuote || ch == singleQuote {
			quote = ch
			continue
		}
		if ch == comment {
			return line[:i]
		}
	}
	return line
}

func tomlSectionName(line string) string {
	if strings.HasPrefix(line, "[[") {
		return ""
	}
	end := strings.Index(line, "]")
	if end < 0 {
		return ""
	}
	return compactTOMLKey(line[1:end])
}

func splitTOMLAssignment(line string) (string, string, bool) {
	if i := strings.Index(line, "="); i >= 0 {
		return line[:i], line[i+1:], true
	}
	return "", "", false
}

func compactTOMLKey(value string) string {
	replacer := strings.NewReplacer(" ", "", "\t", "", "\r", "")
	return replacer.Replace(strings.TrimSpace(value))
}

func inlineTOMLTableHasKey(value, key string) bool {
	compact := compactTOMLKey(value)
	if !strings.HasPrefix(compact, "{") {
		return false
	}
	return strings.Contains(compact, "{"+key+"=") || strings.Contains(compact, ","+key+"=")
}

func validStateRoot(root, home string) bool {
	if root == "" || !filepath.IsAbs(root) {
		return false
	}
	rel, err := filepath.Rel(root, home)
	if err != nil {
		return false
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	rootInfo, err := os.Stat(root)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return false
	}
	match, err := sameFileAsPathOrAncestor(rootInfo, home)
	if err != nil || match {
		return false
	}
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return false
	}
	if resolvedHome == filepath.Clean(home) {
		return true
	}
	match, err = sameFileAsPathOrAncestor(rootInfo, resolvedHome)
	return err == nil && !match
}

func sameFileAsPathOrAncestor(target os.FileInfo, path string) (bool, error) {
	current := filepath.Clean(path)
	for {
		info, err := os.Stat(current)
		if err != nil {
			return false, err
		}
		if os.SameFile(target, info) {
			return true, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}
		current = parent
	}
}

func (c PluginConfig) AllowsProviderFamily(id string) bool {
	if !c.ProviderAllowlistConfigured {
		return true
	}
	for _, enabled := range c.EnabledProviderFamilies {
		if enabled == id {
			return true
		}
	}
	return false
}

func validProfileID(id string) bool {
	return id != "" && !strings.HasPrefix(id, ".") && !strings.Contains(id, "..") &&
		!strings.ContainsAny(id, `/\\`)
}

// ConfigurePluginState applies cfg's resolved state root for all downstream
// packages without making those leaf packages depend on setup.
func ConfigurePluginState(cfg PluginConfig) func() {
	return pluginstate.Configure(cfg.StateDir)
}

// SeedPluginConfigIfMissing writes default config.toml when missing; returns true if created.
func SeedPluginConfigIfMissing(configDir string) bool {
	_ = os.MkdirAll(configDir, 0o755)
	path := PluginConfigPath(configDir)
	if _, err := os.Stat(path); err == nil {
		return false
	}
	_ = os.WriteFile(path, []byte(DefaultPluginConfigTOML(DefaultPluginConfig)), 0o644)
	return true
}

// ResolveClaudeProfiles loads the plugin config and resolves its
// [[claude.profiles]] into concrete profiles (synthesizing the single implicit
// default when none are configured). Shared by the write side (statusLine
// routing) and the read side (panel/sidebar/notify).
func ResolveClaudeProfiles(env map[string]string) []claude.ClaudeProfile {
	cfg := LoadPluginConfig(ResolvePluginConfigDir(env))
	home, _ := os.UserHomeDir()
	return claude.ResolveProfiles(cfg.ClaudeProfiles, env, home)
}

// ResolveActiveClaudeProfile resolves the configured profiles and picks the one
// this process is running as, using its own CLAUDE_CONFIG_DIR. Write side only
// (statusLine): the read side cannot see that env var. The full profile list is
// returned alongside so a caller can report an unmatched config dir.
func ResolveActiveClaudeProfile(env map[string]string) (claude.ClaudeProfile, []claude.ClaudeProfile, bool) {
	profiles := ResolveClaudeProfiles(env)
	home, _ := os.UserHomeDir()
	profile, ok := claude.ResolveActiveProfile(profiles, env["CLAUDE_CONFIG_DIR"], home)
	return profile, profiles, ok
}

// ResolveCodexProfiles loads the plugin config and resolves its
// [[codex.profiles]] into concrete profiles (synthesizing the single implicit
// default when none are configured).
func ResolveCodexProfiles(env map[string]string) []codex.CodexProfile {
	cfg := LoadPluginConfig(ResolvePluginConfigDir(env))
	home, _ := os.UserHomeDir()
	return codex.ResolveProfiles(cfg.CodexProfiles, env, home)
}

// ResolveActiveCodexProfile picks the profile matching this process's
// CODEX_HOME. Empty CODEX_HOME means the default ~/.codex account.
func ResolveActiveCodexProfile(env map[string]string) (codex.CodexProfile, []codex.CodexProfile, bool) {
	profiles := ResolveCodexProfiles(env)
	home, _ := os.UserHomeDir()
	profile, ok := codex.ResolveActiveProfile(profiles, env["CODEX_HOME"], home)
	return profile, profiles, ok
}

func ResolveGrokProfiles(env map[string]string) []grok.GrokProfile {
	cfg := LoadPluginConfig(ResolvePluginConfigDir(env))
	home, _ := os.UserHomeDir()
	return grok.ResolveProfiles(cfg.GrokProfiles, env, home)
}

func ResolveOpenCodeProfiles(env map[string]string) []opencode.OpenCodeProfile {
	cfg := LoadPluginConfig(ResolvePluginConfigDir(env))
	home, _ := os.UserHomeDir()
	return opencode.ResolveProfiles(cfg.OpenCodeProfiles, env, home)
}

// LoadPluginConfig loads config.toml or returns defaults.
func LoadPluginConfig(configDir string) PluginConfig {
	path := PluginConfigPath(configDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return DefaultPluginConfig
	}
	return ParsePluginConfigTOML(string(raw))
}
