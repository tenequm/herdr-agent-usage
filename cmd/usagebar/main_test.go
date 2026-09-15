// Command-level tests for usagebar notification configuration.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/senna-lang/herdr-agent-usage/internal/limits"
	"github.com/senna-lang/herdr-agent-usage/internal/providers/claude"
	"github.com/senna-lang/herdr-agent-usage/internal/ratelimit"
	"github.com/senna-lang/herdr-agent-usage/internal/setup"
	"github.com/senna-lang/herdr-agent-usage/internal/updatecheck"
)

func TestManifestBuildHookCompilesThisCheckout(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "herdr-plugin.toml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(raw)
	if !strings.Contains(manifest, `command = ["bash", "bin/ensure-binary.sh", "--build"]`) {
		t.Fatal("manifest build hook does not require a source build")
	}
	deadMode := "--in" + "-tree"
	if strings.Contains(manifest, deadMode) {
		t.Fatal("dead legacy download mode remains in the manifest")
	}
	scriptRaw, err := os.ReadFile(filepath.Join("..", "..", "bin", "ensure-binary.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptRaw)
	if !strings.Contains(script, "CGO_ENABLED=0 go build") {
		t.Fatal("source build does not disable cgo")
	}
	if strings.Contains(script, deadMode) {
		t.Fatal("dead legacy download mode remains in ensure-binary.sh")
	}
}

// TestNotificationsEnabledHonorsPluginConfig ensures both notification entrypoints
// use the documented [notify].enabled switch.
func TestNotificationsEnabledHonorsPluginConfig(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[notify]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if notificationsEnabled(map[string]string{"HERDR_PLUGIN_CONFIG_DIR": configDir}) {
		t.Fatal("notifications must be disabled by config")
	}
}

func TestUpdateNotificationHonorsPluginConfig(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[notify]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if updateNotification(map[string]string{"HERDR_PLUGIN_CONFIG_DIR": configDir}) != nil {
		t.Fatal("update toast must be disabled by config")
	}
}

func TestPublishPanelSidebarDisabledWritesNothing(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[ui]\nsidebar = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"limits", "panel"} {
		t.Run(command, func(t *testing.T) {
			calls := 0
			call := func() { calls++ }
			publishPanelSidebarWith(map[string]string{"HERDR_PLUGIN_CONFIG_DIR": configDir}, call, call, call)
			if calls != 0 {
				t.Fatalf("disabled %s publishing made %d calls", command, calls)
			}
		})
	}
}

func TestDisabledPanelCommandsDoNotClearMetadata(t *testing.T) {
	for _, command := range []string{"limits", "panel"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			configDir := filepath.Join(root, "config")
			stateDir := filepath.Join(root, "state")
			if err := os.MkdirAll(configDir, 0o700); err != nil {
				t.Fatal(err)
			}
			config := "[ui]\nsidebar = false\n[providers]\nenabled = []\n[state]\ndir = \"" + stateDir + "\"\n"
			if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}

			callLog := filepath.Join(root, "herdr-calls")
			herdr := filepath.Join(root, "fake-herdr")
			script := `#!/bin/sh
printf "%s\n" "$*" >> "$HERDR_CALL_LOG"
if [ "$1" = pane ] && [ "$2" = list ]; then
  printf "%s\n" "{\"result\":{\"panes\":[{\"pane_id\":\"p1\",\"agent\":\"claude\"}]}}"
else
  printf "%s\n" "{\"result\":{}}"
fi
`
			if err := os.WriteFile(herdr, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}

			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestUsagebarMainHelper$")
			cmd.Env = []string{
				"HOME=" + root,
				"XDG_CONFIG_HOME=" + filepath.Join(root, ".config"),
				"XDG_STATE_HOME=" + filepath.Join(root, ".local", "state"),
				"HERDR_PLUGIN_CONFIG_DIR=" + configDir,
				"HERDR_BIN_PATH=" + herdr,
				"HERDR_CALL_LOG=" + callLog,
				"USAGEBAR_MAIN_HELPER=" + command,
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s helper: %v\n%s", command, err, stderr.String())
			}
			raw, err := os.ReadFile(callLog)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "report-metadata") {
				t.Fatalf("disabled %s issued metadata writes:\n%s", command, raw)
			}
		})
	}
}

func TestUsagebarMainHelper(t *testing.T) {
	command := os.Getenv("USAGEBAR_MAIN_HELPER")
	if command == "" {
		return
	}
	os.Args = []string{"usagebar", command, "--once"}
	main()
}

func TestSidebarCommandGates(t *testing.T) {
	for _, command := range []string{"status", "update", "startup", "watch"} {
		t.Run(command, func(t *testing.T) {
			calls := 0
			clears := 0
			actions := sidebarCommandActions{
				update:         func(bool) { calls++ },
				startup:        func() { calls++ },
				startIdleWatch: func() { calls++ },
				watch:          func() { calls++ },
				clearAll:       func() { clears++ },
			}
			if !dispatchSidebarCommand(command, nil, setup.PluginConfig{Sidebar: false}, nil, actions) {
				t.Fatalf("%s was not handled by sidebar dispatch", command)
			}
			if calls != 0 {
				t.Fatalf("disabled %s made %d publishing calls", command, calls)
			}
			wantClears := 0
			if command == "startup" {
				wantClears = 1
			}
			if clears != wantClears {
				t.Fatalf("disabled %s clears=%d, want %d", command, clears, wantClears)
			}
		})
	}
}

func TestSidebarDefaultRunsActionsAndPanePublishing(t *testing.T) {
	if !setup.DefaultPluginConfig.Sidebar {
		t.Fatal("sidebar default must remain enabled")
	}
	updates := 0
	watches := 0
	if !dispatchSidebarCommand("update", []string{"--force"}, setup.DefaultPluginConfig, nil, sidebarCommandActions{
		update: func(force bool) {
			if !force {
				t.Fatal("--force was ignored")
			}
			updates++
		},
		startIdleWatch: func() { watches++ },
	}) {
		t.Fatal("update was not handled")
	}
	if updates != 1 || watches != 1 {
		t.Fatalf("updates=%d watches=%d", updates, watches)
	}
}

func TestLimitsPaneEmptyMessageModes(t *testing.T) {
	activeOnly, emptyMessage := limitsPaneMode(nil, limits.CollectOptions{})
	if !activeOnly || emptyMessage != "(no agent panes open)" {
		t.Fatalf("default mode = (%t, %q)", activeOnly, emptyMessage)
	}

	activeOnly, emptyMessage = limitsPaneMode(nil, limits.CollectOptions{AllowedFamilies: map[string]bool{"claude": true}})
	if activeOnly || strings.Contains(emptyMessage, "pane") {
		t.Fatalf("allowlist mode = (%t, %q)", activeOnly, emptyMessage)
	}
}

func TestRunUpdateCheck_AutoDisabledMakesNoRequest(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[update]\nauto_check = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runUpdateCheckWith(
		[]string{"--quiet", "--current-version", "1.0.0"},
		map[string]string{"HERDR_PLUGIN_CONFIG_DIR": configDir},
		func(updatecheck.Options) updatecheck.Result { calls++; return updatecheck.Result{} },
	)
	if calls != 0 {
		t.Fatalf("disabled automatic check made %d requests", calls)
	}
}

func TestRunUpdateCheck_ExplicitStillRunsWhenAutoDisabled(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[update]\nauto_check = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	runUpdateCheckWith(
		[]string{"--force", "--current-version", "1.0.0"},
		map[string]string{"HERDR_PLUGIN_CONFIG_DIR": configDir},
		func(updatecheck.Options) updatecheck.Result { calls++; return updatecheck.Result{Current: "1.0.0"} },
	)
	if calls != 1 {
		t.Fatalf("explicit check calls=%d, want 1", calls)
	}
}

func TestStatusLineProfilesUseConfiguredStateRoot(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "plugin")
	stateRoot := filepath.Join(home, "plugin-state")
	profileA := filepath.Join(home, ".claude-work")
	profileB := filepath.Join(home, ".claude-personal")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "[notify]\nenabled = false\n[state]\ndir = \"" + stateRoot + "\"\n" +
		"[providers]\nenabled = [\"claude\"]\n" +
		"[[claude.profiles]]\nid = \"work\"\nconfig_dir = \"" + profileA + "\"\n" +
		"[[claude.profiles]]\nid = \"personal\"\nconfig_dir = \"" + profileB + "\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_STATE_HOME":          filepath.Join(home, ".local", "state"),
		"HERDR_PLUGIN_CONFIG_DIR": configDir, "HERDR_ENV": "", "HERDR_SOCKET_PATH": "",
		"HERDR_BIN_PATH": "", "HERDR_PANE_ID": "", "HERDR_TAB_ID": "", "HERDR_WORKSPACE_ID": "",
		"USAGEBAR_STATE_DIR": "", "USAGEBAR_CLAUDE_LIMITS_PATH": "",
	} {
		t.Setenv(key, value)
	}
	cfg := setup.LoadPluginConfig(configDir)
	restore := setup.ConfigurePluginState(cfg)
	defer restore()

	payloadA := `{"rate_limits":{"five_hour":{"used_percentage":10,"resets_at":1800000000}}}`
	payloadB := `{"rate_limits":{"five_hour":{"used_percentage":20,"resets_at":1800000000}}}`
	t.Setenv("CLAUDE_CONFIG_DIR", profileA)
	runStatusLineInput(payloadA, 1_700_000_000_000)
	t.Setenv("CLAUDE_CONFIG_DIR", profileB)
	runStatusLineInput(payloadB, 1_700_000_001_000)

	opts := limits.DefaultCollectOptions()
	if len(opts.Claude) != 2 {
		t.Fatalf("Claude collectors = %d", len(opts.Claude))
	}
	for i, wantUsed := range []float64{10, 20} {
		got := opts.Claude[i].Collector(nil, 1_700_000_002_000)
		if got.Primary == nil || got.Primary.UsedPercentage != wantUsed {
			t.Fatalf("profile %d limits = %+v", i, got.Primary)
		}
		cache := filepath.Join(stateRoot, "claude", opts.Claude[i].ID, "claude-limits-latest.json")
		info, err := os.Stat(cache)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("cache mode = %o", info.Mode().Perm())
		}
		if dirInfo, err := os.Stat(filepath.Dir(cache)); err != nil || dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("profile dir mode = %v, %v", dirInfo, err)
		}
	}
	for _, forbidden := range []string{filepath.Join(home, ".claude"), profileA, profileB, filepath.Join(home, ".cursor")} {
		if _, err := os.Stat(forbidden); !os.IsNotExist(err) {
			t.Fatalf("created state under harness path %s", forbidden)
		}
	}
}

func TestStatusLineDiscoversConfiguredStateRootWithoutHerdrEnv(t *testing.T) {
	tests := []struct {
		name   string
		useXDG bool
	}{
		{name: "XDG config home", useXDG: true},
		{name: "HOME fallback", useXDG: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			for _, key := range []string{
				"HERDR_ENV", "HERDR_SOCKET_PATH", "HERDR_BIN_PATH", "HERDR_PANE_ID",
				"HERDR_TAB_ID", "HERDR_WORKSPACE_ID", "HERDR_PLUGIN_CONFIG_DIR",
				"HERDR_PLUGIN_ACTION_ID", "CLAUDE_CONFIG_DIR", "USAGEBAR_STATE_DIR",
				"USAGEBAR_CLAUDE_LIMITS_PATH",
			} {
				unsetEnv(t, key)
			}
			t.Setenv("HOME", home)
			configHome := filepath.Join(home, ".config")
			if tt.useXDG {
				configHome = filepath.Join(home, "xdg-config")
				t.Setenv("XDG_CONFIG_HOME", configHome)
			} else {
				unsetEnv(t, "XDG_CONFIG_HOME")
			}
			configDir := filepath.Join(configHome, "herdr", "plugins", "config", "usagebar")
			stateRoot := filepath.Join(home, "plugin-state")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			raw := "[notify]\nenabled = false\n[state]\ndir = \"" + stateRoot + "\"\n"
			if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}

			config, restore := configureRuntime(environment())
			defer restore()
			if config.StateDir != stateRoot {
				t.Fatalf("state dir = %q, want %q", config.StateDir, stateRoot)
			}
			runStatusLineInput(`{"rate_limits":{"five_hour":{"used_percentage":10,"resets_at":1800000000}}}`, 1_700_000_000_000)
			cache := filepath.Join(stateRoot, "claude", "claude", "claude-limits-latest.json")
			if _, err := os.Stat(cache); err != nil {
				t.Fatalf("statusline cache: %v", err)
			}
			if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
				t.Fatalf("statusline wrote under legacy harness dir: %v", err)
			}
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, present := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if present {
			_ = os.Setenv(key, value)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// TestStatusLineNotificationsDeduplicatesEveryTick reproduces issue #32's
// once-per-second statusLine calls after entering the 50% remaining bucket.
func TestStatusLineNotificationsDeduplicatesEveryTick(t *testing.T) {
	profile := claude.ClaudeProfile{StateDir: t.TempDir()}
	config := setup.PluginConfig{NotifyEnabled: true, RemainingThresholds: []int{50, 20, 10, 5}}
	payload := `{"rate_limits":{"five_hour":{"used_percentage":55,"resets_at":1800000000}}}`
	notifications := 0
	notify := ratelimit.ShowNotificationFn(func(_, _ string) bool {
		notifications++
		return true
	})

	for nowMs := int64(1_700_000_000_000); nowMs < 1_700_000_003_000; nowMs += 1_000 {
		runStatusLineNotifications(profile, payload, nowMs, config, notify)
	}

	if notifications != 1 {
		t.Fatalf("statusline sent %d notifications, want exactly one", notifications)
	}
	raw, err := os.ReadFile(filepath.Join(profile.StateDir, "rate-limit-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		FiveHour *struct {
			NotifiedBucket *string `json:"notifiedBucket"`
		} `json:"fiveHour"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if state.FiveHour == nil || state.FiveHour.NotifiedBucket == nil || *state.FiveHour.NotifiedBucket != "50" {
		t.Fatalf("notified bucket was not persisted: %s", raw)
	}
}

// TestStatusLineNotificationsDisabled reproduces issue #32's documented
// notify.enabled=false switch for the same threshold-crossing statusline input.
func TestStatusLineNotificationsDisabled(t *testing.T) {
	profile := claude.ClaudeProfile{StateDir: t.TempDir()}
	config := setup.PluginConfig{NotifyEnabled: false, RemainingThresholds: []int{50, 20, 10, 5}}
	notifications := 0

	runStatusLineNotifications(
		profile,
		`{"rate_limits":{"five_hour":{"used_percentage":55,"resets_at":1800000000}}}`,
		1_700_000_000_000,
		config,
		func(_, _ string) bool {
			notifications++
			return true
		},
	)

	if notifications != 0 {
		t.Fatalf("disabled notifications sent %d toasts", notifications)
	}
}

func TestLimitsPaneEmptyMessageInvalidAllowlist(t *testing.T) {
	activeOnly, message := limitsPaneMode(nil, limits.CollectOptions{AllowedFamilies: map[string]bool{}})
	if activeOnly || !strings.Contains(message, "[providers].enabled") {
		t.Fatalf("mode = (%t, %q)", activeOnly, message)
	}
}

func TestOpenCodeCheckExplainsExcludedFamily(t *testing.T) {
	var out bytes.Buffer
	if allowOpenCodeCheck(limits.CollectionBound{Configured: true, Families: map[string]bool{}}, &out) {
		t.Fatal("excluded OpenCode check was allowed")
	}
	if got := strings.TrimSpace(out.String()); got != "opencode is not in [providers].enabled" {
		t.Fatalf("message = %q", got)
	}
}
