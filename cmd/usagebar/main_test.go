// Command-level tests for usagebar notification configuration.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/senna-lang/herdr-agent-usage/internal/providers/claude"
	"github.com/senna-lang/herdr-agent-usage/internal/ratelimit"
	"github.com/senna-lang/herdr-agent-usage/internal/setup"
	"github.com/senna-lang/herdr-agent-usage/internal/updatecheck"
)

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
	calls := 0
	call := func() { calls++ }
	publishPanelSidebarWith(false, call, call, call)
	if calls != 0 {
		t.Fatalf("disabled pane publishing made %d calls", calls)
	}
}

func TestSidebarCommandGates(t *testing.T) {
	for _, command := range []string{"status", "update", "startup", "watch"} {
		t.Run(command, func(t *testing.T) {
			calls := 0
			runSidebarActions(false, func() { calls++ }, func() { calls++ })
			if calls != 0 {
				t.Fatalf("disabled %s made %d calls", command, calls)
			}
		})
	}
}

func TestSidebarDefaultRunsActionsAndPanePublishing(t *testing.T) {
	if !setup.DefaultPluginConfig.Sidebar {
		t.Fatal("sidebar default must remain enabled")
	}
	calls := 0
	call := func() { calls++ }
	runSidebarActions(setup.DefaultPluginConfig.Sidebar, call, call)
	publishPanelSidebarWith(setup.DefaultPluginConfig.Sidebar, call, call, call)
	if calls != 5 {
		t.Fatalf("default sidebar made %d calls, want 5", calls)
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
