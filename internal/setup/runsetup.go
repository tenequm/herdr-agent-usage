/**
 * Orchestrates the usagebar setup.
 * - Seed the plugin config
 * - Inspect the Herdr toast config
 * - Optionally append the toast config (--write-toast)
 * - Print snippets to paste
 */
package setup

import (
	"os"
	"strings"

	"github.com/senna-lang/herdr-agent-usage/internal/pluginstate"
)

// SetupOptions configures runSetup.
type SetupOptions struct {
	// WriteToast appends toast config when missing.
	WriteToast bool
	Env        map[string]string
}

// SetupReport is the setup command output.
type SetupReport struct {
	Lines              []string
	PluginConfigSeeded bool
	ToastWrote         bool
}

// RunSetup seeds plugin config, optionally writes toast, and prints snippets.
func RunSetup(options SetupOptions) SetupReport {
	env := options.Env
	if env == nil {
		env = ProcessEnv()
	}
	var lines []string
	pluginDir := ResolvePluginConfigDir(env)
	herdrConfigPath := ResolveHerdrConfigPath(env)

	lines = append(lines, "Agent Usage · setup", "──────────────────")

	seeded := SeedPluginConfigIfMissing(pluginDir)
	pluginPath := PluginConfigPath(pluginDir)
	pluginCfg := LoadPluginConfig(pluginDir)
	restoreState := ConfigurePluginState(pluginCfg)
	defer restoreState()
	home, _ := os.UserHomeDir()
	if seeded {
		lines = append(lines, "✓ seeded plugin config: "+pluginPath)
	} else {
		lines = append(lines, "· plugin config exists: "+pluginPath)
	}
	thr := make([]string, len(pluginCfg.RemainingThresholds))
	for i, n := range pluginCfg.RemainingThresholds {
		thr[i] = itoa(n)
	}
	lines = append(lines,
		"  notify.enabled="+boolStr(pluginCfg.NotifyEnabled)+"  thresholds=["+strings.Join(thr, ", ")+"]",
		"  ui.sidebar="+boolStr(pluginCfg.Sidebar),
		"  ui.account_email="+boolStr(pluginCfg.AccountEmail),
		"  update.auto_check="+boolStr(pluginCfg.AutoCheck),
	)
	switch {
	case !pluginCfg.ProviderAllowlistConfigured:
		lines = append(lines, "  providers.enabled=[] (all provider families)")
	case len(pluginCfg.EnabledProviderFamilies) > 0:
		lines = append(lines, "  providers.enabled=["+strings.Join(pluginCfg.EnabledProviderFamilies, ", ")+"]")
	default:
		lines = append(lines,
			"  providers.enabled=[] (NO provider families)",
			"  ! providers.enabled has no supported family; all collection is disabled",
		)
	}
	for _, id := range pluginCfg.UnknownProviderFamilies {
		lines = append(lines, "  ! unknown provider family ignored: "+id)
	}
	if pluginCfg.DecodeError != "" {
		lines = append(lines, "  ! config decode error; security-sensitive settings failed closed: "+pluginCfg.DecodeError)
	}
	lines = append(lines, "  state.dir="+pluginstate.GlobalDir(home))
	if pluginCfg.InvalidStateDir != "" {
		lines = append(lines, "  ! state.dir ignored (must be absolute and narrower than the home directory): "+pluginCfg.InvalidStateDir)
	}
	for _, id := range pluginCfg.InvalidProfileIDs {
		lines = append(lines, "  ! invalid profile id ignored: "+id)
	}
	excludedFamilies := make([]string, 0, 4)
	if pluginCfg.AllowsProviderFamily("claude") {
		lines = append(lines, claudeProfileReportLines(pluginCfg.ClaudeProfiles, ResolveClaudeProfiles(env), home)...)
	} else {
		excludedFamilies = append(excludedFamilies, "claude")
	}
	if pluginCfg.AllowsProviderFamily("codex") {
		lines = append(lines, codexProfileReportLines(pluginCfg.CodexProfiles, ResolveCodexProfiles(env), home)...)
	} else {
		excludedFamilies = append(excludedFamilies, "codex")
	}
	if pluginCfg.AllowsProviderFamily("grok") {
		lines = append(lines, grokProfileReportLines(pluginCfg.GrokProfiles, ResolveGrokProfiles(env), home)...)
	} else {
		excludedFamilies = append(excludedFamilies, "grok")
	}
	if pluginCfg.AllowsProviderFamily("opencode") {
		lines = append(lines, openCodeProfileReportLines(pluginCfg.OpenCodeProfiles, ResolveOpenCodeProfiles(env), env, home)...)
	} else {
		excludedFamilies = append(excludedFamilies, "opencode")
	}
	if len(excludedFamilies) > 0 {
		lines = append(lines, "· profiles not enabled: "+strings.Join(excludedFamilies, ", "), "")
	}

	toastWrote := false
	if options.WriteToast {
		result := AppendToastConfigIfMissing(herdrConfigPath)
		toastWrote = result.Wrote
		if result.Wrote {
			lines = append(lines, "✓ "+result.Reason, "  run: herdr server reload-config", "")
		} else {
			lines = append(lines, "· "+result.Reason, "")
		}
	}

	toastStatusText := "Herdr config not found yet"
	if _, err := os.Stat(herdrConfigPath); err == nil {
		raw, _ := os.ReadFile(herdrConfigPath)
		status := InspectToastConfig(string(raw))
		switch {
		case status.Kind == "missing":
			toastStatusText = "toast: NOT configured (notifications may not appear)"
		case status.Delivery == nil:
			toastStatusText = "toast: section present, delivery not set"
		default:
			toastStatusText = "toast: delivery=" + *status.Delivery
		}
	}
	lines = append(lines,
		"Herdr config: "+herdrConfigPath,
		"  "+toastStatusText,
		"",
		"── Paste into ~/.config/herdr/config.toml ──",
		"",
	)
	if pluginCfg.Sidebar {
		lines = append(lines,
			"# Sidebar context + provider limit rows (Herdr 0.7.4+)",
			"# If [ui.sidebar.agents] already exists, merge these rows; do not add a duplicate table.",
			strings.TrimRight(SidebarRowsSnippet(), "\n"),
			"",
		)
	} else {
		lines = append(lines, "# Sidebar disabled: pane-only mode; use the prefix keybinding for the limits overlay.", "")
	}
	lines = append(lines,
		"# Toast delivery (required for rate-limit notifications)",
		strings.TrimRight(ToastConfigSnippet(), "\n"),
		"",
		"# Optional keybindings",
		strings.TrimRight(KeybindingSnippet(), "\n"),
		"",
		"Then: herdr server reload-config",
		"",
	)
	if !options.WriteToast {
		lines = append(lines,
			"Tip: run with --write-toast to append the toast block automatically",
			"     (only if [ui.toast] is missing; never overwrites).",
			"",
		)
	}
	root := env["HERDR_PLUGIN_ROOT"]
	if root == "" {
		root = "/path/to/herdr-agent-usage"
	}
	lines = append(lines,
		"Claude statusLine (optional, for CC rate windows):",
		`  "command": "bash `+root+`/bin/run-statusline.sh"`,
		"",
	)
	lines = append(lines, cursorSetupLines(root)...)

	return SetupReport{Lines: lines, PluginConfigSeeded: seeded, ToastWrote: toastWrote}
}

func ProcessEnv() map[string]string {
	env := map[string]string{}
	for _, e := range os.Environ() {
		if i := strings.IndexByte(e, '='); i >= 0 {
			env[e[:i]] = e[i+1:]
		}
	}
	return env
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
