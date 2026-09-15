/**
 * Tests for RunSetup
 */
package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestRunSetup_SeedsAndSnippets(t *testing.T) {
	pluginDir := t.TempDir()
	herdrDir := t.TempDir()
	herdrConfig := filepath.Join(herdrDir, "config.toml")
	if err := os.WriteFile(herdrConfig, []byte("[theme]\nname = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := RunSetup(SetupOptions{
		WriteToast: false,
		Env: map[string]string{
			"HERDR_PLUGIN_CONFIG_DIR": pluginDir,
			"HERDR_CONFIG":            herdrConfig,
			"HERDR_PLUGIN_ROOT":       "/tmp/plugin-root",
		},
	})
	if !report.PluginConfigSeeded || report.ToastWrote {
		t.Fatalf("seeded=%v toast=%v", report.PluginConfigSeeded, report.ToastWrote)
	}
	text := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"seeded plugin config", "toast: NOT configured", "[ui.toast]",
		"ui.account_email=false", "[ui.sidebar.agents]", `["state_icon", "$title"]`,
		`"$limit"`, `"$cache_high"`, `"$cache_mid"`, `"$cache_low"`, `"$context"`,
		"usagebar.open-limits", "--write-toast",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestRunSetupReportsAccountEmailEnabled(t *testing.T) {
	pluginDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(pluginDir, "config.toml"),
		[]byte("[ui]\naccount_email = true\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	report := RunSetup(SetupOptions{Env: map[string]string{
		"HERDR_PLUGIN_CONFIG_DIR": pluginDir,
		"HERDR_CONFIG":            filepath.Join(t.TempDir(), "config.toml"),
	}})
	if text := strings.Join(report.Lines, "\n"); !strings.Contains(text, "ui.account_email=true") {
		t.Fatalf("setup did not report enabled account email:\n%s", text)
	}
}

func TestRunSetup_WriteToast(t *testing.T) {
	pluginDir := t.TempDir()
	herdrDir := t.TempDir()
	herdrConfig := filepath.Join(herdrDir, "config.toml")
	if err := os.WriteFile(herdrConfig, []byte("[theme]\nname = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := RunSetup(SetupOptions{
		WriteToast: true,
		Env: map[string]string{
			"HERDR_PLUGIN_CONFIG_DIR": pluginDir,
			"HERDR_CONFIG":            herdrConfig,
		},
	})
	if !report.ToastWrote {
		t.Fatal("expected toast write")
	}
	if !strings.Contains(strings.Join(report.Lines, "\n"), "appended toast config") {
		t.Fatal("missing append message")
	}
}

func TestRunSetup_InvalidProviderAllowlistReportsCollectionDisabled(t *testing.T) {
	pluginDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(pluginDir, "config.toml"),
		[]byte("[providers]\nenabled = [\"typo\", \"omp\"]\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	report := RunSetup(SetupOptions{Env: map[string]string{
		"HERDR_PLUGIN_CONFIG_DIR": pluginDir,
		"HERDR_CONFIG":            filepath.Join(t.TempDir(), "config.toml"),
	}})
	text := strings.Join(report.Lines, "\n")
	for _, want := range []string{
		"providers.enabled=[] (NO provider families)",
		"all collection is disabled",
		"unknown provider family ignored: typo",
		"unknown provider family ignored: omp",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestRunSetupPaneOnlyPasteBlockIsValidTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pluginDir := filepath.Join(home, "plugin-config")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "config.toml"), []byte("[ui]\nsidebar = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := RunSetup(SetupOptions{Env: map[string]string{
		"HOME":                    home,
		"XDG_CONFIG_HOME":         filepath.Join(home, ".config"),
		"XDG_STATE_HOME":          filepath.Join(home, ".local", "state"),
		"HERDR_PLUGIN_CONFIG_DIR": pluginDir,
		"HERDR_CONFIG":            filepath.Join(home, "herdr-config.toml"),
	}})
	text := strings.Join(report.Lines, "\n")
	startMarker := "── Paste into ~/.config/herdr/config.toml ──"
	start := strings.Index(text, startMarker)
	if start < 0 {
		t.Fatalf("paste block header missing:\n%s", text)
	}
	start += len(startMarker)
	end := strings.Index(text[start:], "\nThen: herdr server reload-config")
	if end < 0 {
		t.Fatalf("paste block footer missing:\n%s", text)
	}
	block := text[start : start+end]
	var parsed map[string]any
	if _, err := toml.Decode(block, &parsed); err != nil {
		t.Fatalf("pane-only paste block is invalid TOML: %v\n%s", err, block)
	}
}

func TestRunSetupPaneOnlyOmitsSidebarAndExcludedProfiles(t *testing.T) {
	pluginDir := t.TempDir()
	raw := "[ui]\nsidebar = false\n[providers]\nenabled = [\"claude\"]\n" +
		"[[grok.profiles]]\nid = \"work\"\ngrok_home = \"/must-not-read\"\n" +
		"[[opencode.profiles]]\nid = \"work\"\ndata_dir = \"/must-not-read\"\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "config.toml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	report := RunSetup(SetupOptions{Env: map[string]string{
		"HERDR_PLUGIN_CONFIG_DIR": pluginDir,
		"HERDR_CONFIG":            filepath.Join(t.TempDir(), "config.toml"),
	}})
	text := strings.Join(report.Lines, "\n")
	if strings.Contains(text, "[ui.sidebar.agents]") || strings.Contains(text, "/must-not-read") {
		t.Fatalf("disabled or excluded setup content leaked:\n%s", text)
	}
	for _, want := range []string{"Sidebar disabled: pane-only mode", "profiles not enabled: codex, grok, opencode"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}
