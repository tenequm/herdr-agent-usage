/**
 * Tests for CollectAllProviderLimits facade.
 */
package limits

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectAllProviderLimits_OrderAndStubs(t *testing.T) {
	got := CollectAllProviderLimits(nil, 100, CollectOptions{})
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	wantIDs := []string{"claude", "codex", "opencode", "grok"}
	for i, id := range wantIDs {
		if got[i].ProviderID != id {
			t.Fatalf("[%d] id=%q want %q", i, got[i].ProviderID, id)
		}
		if got[i].Note == nil || !containsStr(*got[i].Note, "not configured") {
			t.Fatalf("[%d] expected stub note, got %v", i, got[i].Note)
		}
	}
}

func TestCollectAllProviderLimits_WithCollectorsAndAttach(t *testing.T) {
	cwd := "/tmp"
	got := CollectAllProviderLimits(&cwd, 200, CollectOptions{
		Claude: []ClaudeProfileCollector{{
			ID: "claude", Label: "Claude",
			Collector: func(c *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "claude", Label: "Claude", Source: "test", FetchedAtMs: now}
			},
		}},
		Codex: []CodexProfileCollector{{
			ID: "codex", Label: "Codex",
			Collector: func(c *string, now int64) ProviderLimits {
				if c == nil || *c != "/tmp" {
					t.Fatal("cwd not passed")
				}
				return ProviderLimits{ProviderID: "codex", Label: "Codex", Source: "test", FetchedAtMs: now}
			},
		}},
		Attach: func(providers []ProviderLimits, nowMs int64) []ProviderLimits {
			if nowMs != 200 || len(providers) != 4 {
				t.Fatalf("attach args")
			}
			providers[0].PaneActivity = &ProviderPaneActivity{WindowMinutes: 300, TotalTokens: 1}
			return providers
		},
	})
	if got[0].PaneActivity == nil || got[0].PaneActivity.TotalTokens != 1 {
		t.Fatalf("attach not applied: %+v", got[0])
	}
	if got[1].Source != "test" {
		t.Fatalf("codex=%+v", got[1])
	}
}

func TestCollectAllProviderLimits_OnlyFiltersProviders(t *testing.T) {
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Only: map[string]bool{"claude": true, "grok": true},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].ProviderID != "claude" || got[1].ProviderID != "grok" {
		t.Fatalf("ids=%q,%q want claude,grok (display order kept)", got[0].ProviderID, got[1].ProviderID)
	}
}

func TestCollectAllProviderLimits_OnlyEmptyHidesAll(t *testing.T) {
	got := CollectAllProviderLimits(nil, 100, CollectOptions{Only: map[string]bool{}})
	if len(got) != 0 {
		t.Fatalf("len=%d, want 0", len(got))
	}
}

func TestCollectAllProviderLimits_OnlySkipsFilteredCollectors(t *testing.T) {
	codexCalled := false
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Only: map[string]bool{"claude": true},
		Codex: []CodexProfileCollector{{
			ID: "codex", Label: "Codex",
			Collector: func(_ *string, now int64) ProviderLimits {
				codexCalled = true
				return ProviderLimits{ProviderID: "codex", Label: "Codex", Source: "test", FetchedAtMs: now}
			},
		}},
		Attach: func(providers []ProviderLimits, _ int64) []ProviderLimits {
			if len(providers) != 1 {
				t.Fatalf("attach got %d providers, want 1 (filtered)", len(providers))
			}
			return providers
		},
	})
	if codexCalled {
		t.Fatal("codex collector ran despite being filtered out")
	}
	if len(got) != 1 || got[0].ProviderID != "claude" {
		t.Fatalf("got %+v", got)
	}
}

func TestCollectAllProviderLimits_AllowedCannotBeOverriddenByOnly(t *testing.T) {
	called := false
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Grok: []GrokProfileCollector{{ID: "grok", Collector: func(_ *string, _ int64) ProviderLimits {
			called = true
			return ProviderLimits{ProviderID: "grok"}
		}}},
		Allowed: map[string]bool{"claude": true},
		Only:    map[string]bool{"claude": true, "grok": true},
	})
	if called {
		t.Fatal("globally excluded collector ran")
	}
	if len(got) != 1 || got[0].ProviderID != "claude" {
		t.Fatalf("collected = %+v", got)
	}
}

func TestDefaultCollectOptions_ExpandsEnabledFamiliesToProfileIDs(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", configDir)
	t.Setenv("HOME", t.TempDir())
	raw := "[providers]\nenabled = [\"claude\", \"codex\"]\n" +
		"[[claude.profiles]]\nid = \"primary\"\nconfig_dir = \"" + t.TempDir() + "\"\n" +
		"[[claude.profiles]]\nid = \"secondary\"\nconfig_dir = \"" + t.TempDir() + "\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := DefaultCollectOptions()
	want := map[string]bool{"primary": true, "secondary": true, "codex": true}
	if len(opts.Allowed) != len(want) {
		t.Fatalf("allowed = %v, want %v", opts.Allowed, want)
	}
	for id := range want {
		if !opts.Allowed[id] {
			t.Fatalf("allowed = %v, missing %q", opts.Allowed, id)
		}
	}
	if opts.Allowed["grok"] || opts.Allowed["opencode"] {
		t.Fatalf("excluded family expanded: %v", opts.Allowed)
	}
}

func TestDefaultCollectOptions_InvalidAllowlistCollectsNothing(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", configDir)
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(
		filepath.Join(configDir, "config.toml"),
		[]byte("[providers]\nenabled = [\"typo\", \"omp\", \"cursor\"]\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	opts := DefaultCollectOptions()
	if opts.Allowed == nil || len(opts.Allowed) != 0 {
		t.Fatalf("allowed = %#v, want configured empty set", opts.Allowed)
	}
	for _, collectors := range [][]ClaudeProfileCollector{opts.Claude, opts.Codex, opts.Grok, opts.OpenCode} {
		for i := range collectors {
			collectors[i].Collector = func(*string, int64) ProviderLimits {
				t.Fatal("collector ran for invalid allowlist")
				return ProviderLimits{}
			}
		}
	}
	if got := CollectAllProviderLimits(nil, 0, opts); len(got) != 0 {
		t.Fatalf("collected = %+v", got)
	}
}

func TestCollectOptions_FilterAllowedPanesBeforeAdapters(t *testing.T) {
	opts := CollectOptions{
		Claude:  []ClaudeProfileCollector{{ID: "claude"}},
		Allowed: map[string]bool{"claude": true},
	}
	got := opts.FilterAllowedPanes([]OpenPaneSnapshot{
		{PaneID: "c", Agent: "claude"},
		{PaneID: "g", Agent: "grok"},
		{PaneID: "o", Agent: "omp"},
	})
	if len(got) != 1 || got[0].PaneID != "c" {
		t.Fatalf("filtered panes = %+v", got)
	}
}

func TestCollectAllProviderLimits_MultipleClaudeProfiles(t *testing.T) {
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Claude: []ClaudeProfileCollector{
			{ID: "claude", Label: "Claude", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "claude", Label: "Claude", Source: "test-a", FetchedAtMs: now}
			}},
			{ID: "claude-secondary", Label: "Claude (secondary)", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "claude-secondary", Label: "Claude (secondary)", Source: "test-b", FetchedAtMs: now}
			}},
		},
		Only: map[string]bool{"claude": true, "claude-secondary": true, "codex": true, "opencode": true, "grok": true},
	})
	if len(got) != 5 {
		t.Fatalf("len=%d want 5: %+v", len(got), got)
	}
	if got[0].ProviderID != "claude" || got[0].Source != "test-a" {
		t.Fatalf("profile 1 = %+v", got[0])
	}
	if got[1].ProviderID != "claude-secondary" || got[1].Source != "test-b" {
		t.Fatalf("profile 2 = %+v", got[1])
	}
	if got[2].ProviderID != "codex" {
		t.Fatalf("codex should follow all claude profiles, got %+v", got[2])
	}
}

func TestCollectAllProviderLimits_ClaudeProfileFilteredByOnly(t *testing.T) {
	secondaryCalled := false
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Claude: []ClaudeProfileCollector{
			{ID: "claude", Label: "Claude", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "claude", Label: "Claude", Source: "test-a", FetchedAtMs: now}
			}},
			{ID: "claude-secondary", Label: "Claude (secondary)", Collector: func(_ *string, now int64) ProviderLimits {
				secondaryCalled = true
				return ProviderLimits{ProviderID: "claude-secondary", Label: "Claude (secondary)", Source: "test-b", FetchedAtMs: now}
			}},
		},
		Only: map[string]bool{"claude": true},
	})
	if len(got) != 1 || got[0].ProviderID != "claude" {
		t.Fatalf("got %+v", got)
	}
	if secondaryCalled {
		t.Fatal("filtered-out profile's collector must not run")
	}
}

func TestDefaultCollectOptions_MultiProfileGroupsEveryProfileUnderClaude(t *testing.T) {
	pluginConfigDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginConfigDir)
	dirLabeled := t.TempDir()
	dirUnlabeled := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirLabeled, ".claude.json"),
		[]byte(`{"oauthAccount":{"emailAddress":"primary@example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirUnlabeled, ".claude.json"),
		[]byte(`{"oauthAccount":{"emailAddress":"secondary@example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	toml := "[[claude.profiles]]\n" +
		"id = \"claude\"\n" +
		"label = \"My Work Account\"\n" +
		"config_dir = \"" + dirLabeled + "\"\n\n" +
		"[[claude.profiles]]\n" +
		"id = \"claude-secondary\"\n" +
		"config_dir = \"" + dirUnlabeled + "\"\n"
	if err := os.WriteFile(filepath.Join(pluginConfigDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultCollectOptions()
	if len(opts.Claude) != 2 {
		t.Fatalf("want 2 claude collectors, got %d", len(opts.Claude))
	}
	// The explicit label must be preserved as Label; AccountLabel carries the
	// real email so the panel can show it instead once grouped.
	if opts.Claude[0].Label != "My Work Account" {
		t.Fatalf("explicit label must be preserved, got %q", opts.Claude[0].Label)
	}
	got0 := opts.Claude[0].Collector(nil, 0)
	if got0.GroupLabel != "Claude" {
		t.Fatalf("labeled profile should be grouped under Claude, got %q", got0.GroupLabel)
	}
	if got0.AccountLabel != "primary@example.com" {
		t.Fatalf("labeled profile should still resolve AccountLabel from its email, got %q", got0.AccountLabel)
	}
	// The unlabeled profile keeps its id as Label, plus the same grouping.
	if opts.Claude[1].Label != "claude-secondary" {
		t.Fatalf("unlabeled profile label should default to id, got %q", opts.Claude[1].Label)
	}
	got1 := opts.Claude[1].Collector(nil, 0)
	if got1.GroupLabel != "Claude" {
		t.Fatalf("unlabeled profile should be grouped under Claude, got %q", got1.GroupLabel)
	}
	if got1.AccountLabel != "secondary@example.com" {
		t.Fatalf("unlabeled profile should resolve AccountLabel from its email, got %q", got1.AccountLabel)
	}
}

func TestDefaultCollectOptions_SingleProfileNotGrouped(t *testing.T) {
	pluginConfigDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginConfigDir)
	// Isolate from the real machine's ~/.claude.json (age/content varies by
	// machine and would otherwise make this test's Note assertion flaky).
	t.Setenv("HOME", t.TempDir())
	// No [[claude.profiles]] configured -> single synthesized default; grouping
	// only kicks in once there are 2+ profiles to disambiguate.
	opts := DefaultCollectOptions()
	if len(opts.Claude) != 1 {
		t.Fatalf("want 1 (default) claude collector, got %d", len(opts.Claude))
	}
	got := opts.Claude[0].Collector(nil, 0)
	if got.GroupLabel != "" {
		t.Fatalf("single-profile mode should not set GroupLabel, got %q", got.GroupLabel)
	}
	if got.AccountLabel != "" {
		t.Fatalf("single-profile mode should not set AccountLabel, got %q", got.AccountLabel)
	}
}

func TestCollectAllProviderLimits_MultipleCodexProfiles(t *testing.T) {
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Codex: []CodexProfileCollector{
			{ID: "codex", Label: "personal", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "codex", Label: "personal", Source: "test-a", FetchedAtMs: now}
			}},
			{ID: "dev", Label: "product", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "dev", Label: "product", Source: "test-b", FetchedAtMs: now}
			}},
		},
		Only: map[string]bool{"claude": true, "codex": true, "dev": true, "opencode": true, "grok": true},
	})
	if len(got) != 5 {
		t.Fatalf("len=%d want 5: %+v", len(got), got)
	}
	if got[1].ProviderID != "codex" || got[1].Source != "test-a" {
		t.Fatalf("profile 1 = %+v", got[1])
	}
	if got[2].ProviderID != "dev" || got[2].Source != "test-b" {
		t.Fatalf("profile 2 = %+v", got[2])
	}
	if got[3].ProviderID != "opencode" {
		t.Fatalf("opencode should follow all codex profiles, got %+v", got[3])
	}
}

func TestCollectAllProviderLimits_CodexProfileFilteredByOnly(t *testing.T) {
	devCalled := false
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Codex: []CodexProfileCollector{
			{ID: "codex", Label: "personal", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "codex", Label: "personal", Source: "test-a", FetchedAtMs: now}
			}},
			{ID: "dev", Label: "product", Collector: func(_ *string, now int64) ProviderLimits {
				devCalled = true
				return ProviderLimits{ProviderID: "dev", Label: "product", Source: "test-b", FetchedAtMs: now}
			}},
		},
		Only: map[string]bool{"codex": true},
	})
	if len(got) != 1 || got[0].ProviderID != "codex" {
		t.Fatalf("got %+v", got)
	}
	if devCalled {
		t.Fatal("filtered-out profile's collector must not run")
	}
}

func TestDefaultCollectOptions_MultiProfileGroupsEveryProfileUnderCodex(t *testing.T) {
	pluginConfigDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginConfigDir)
	t.Setenv("HOME", t.TempDir())
	dirPersonal := t.TempDir()
	dirDev := t.TempDir()
	toml := "[[codex.profiles]]\n" +
		"id = \"codex\"\n" +
		"label = \"personal\"\n" +
		"codex_home = \"" + dirPersonal + "\"\n\n" +
		"[[codex.profiles]]\n" +
		"id = \"dev\"\n" +
		"codex_home = \"" + dirDev + "\"\n"
	if err := os.WriteFile(filepath.Join(pluginConfigDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := DefaultCollectOptions()
	if len(opts.Codex) != 2 {
		t.Fatalf("want 2 codex collectors, got %d", len(opts.Codex))
	}
	if opts.Codex[0].Label != "personal" {
		t.Fatalf("explicit label must be preserved, got %q", opts.Codex[0].Label)
	}
	got0 := opts.Codex[0].Collector(nil, 0)
	if got0.GroupLabel != "Codex" {
		t.Fatalf("labeled profile should be grouped under Codex, got %q", got0.GroupLabel)
	}
	if got0.AccountLabel != "personal" {
		t.Fatalf("AccountLabel should be the profile label, got %q", got0.AccountLabel)
	}
	if opts.Codex[1].Label != "dev" {
		t.Fatalf("unlabeled profile label should default to id, got %q", opts.Codex[1].Label)
	}
	got1 := opts.Codex[1].Collector(nil, 0)
	if got1.GroupLabel != "Codex" || got1.AccountLabel != "dev" {
		t.Fatalf("unlabeled profile grouping = %q/%q", got1.GroupLabel, got1.AccountLabel)
	}
}

func TestCollectAllProviderLimits_MultipleGrokAndOpenCodeProfiles(t *testing.T) {
	got := CollectAllProviderLimits(nil, 100, CollectOptions{
		Grok: []GrokProfileCollector{
			{ID: "grok-personal", Label: "Personal", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "grok-personal", Label: "Personal", Source: "grok-personal", FetchedAtMs: now}
			}},
			{ID: "grok-work", Label: "Work", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "grok-work", Label: "Work", Source: "grok-work", FetchedAtMs: now}
			}},
		},
		OpenCode: []OpenCodeProfileCollector{
			{ID: "opencode-personal", Label: "Personal", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "opencode-personal", Label: "Personal", Source: "opencode-personal", FetchedAtMs: now}
			}},
			{ID: "opencode-work", Label: "Work", Collector: func(_ *string, now int64) ProviderLimits {
				return ProviderLimits{ProviderID: "opencode-work", Label: "Work", Source: "opencode-work", FetchedAtMs: now}
			}},
		},
		Only: map[string]bool{
			"grok-personal":     true,
			"grok-work":         true,
			"opencode-personal": true,
			"opencode-work":     true,
		},
	})

	if len(got) != 4 {
		t.Fatalf("profiles = %+v", got)
	}
	want := []string{"opencode-personal", "opencode-work", "grok-personal", "grok-work"}
	for i, providerID := range want {
		if got[i].ProviderID != providerID || got[i].Source != providerID {
			t.Fatalf("row %d = %+v, want %q", i, got[i], providerID)
		}
	}
}

func TestCollectAllProviderLimits_FamilyScopedProfileIDs(t *testing.T) {
	claudeCalls, grokCalls := 0, 0
	opts := CollectOptions{
		Claude:          []ClaudeProfileCollector{{ID: "work", Collector: func(*string, int64) ProviderLimits { claudeCalls++; return ProviderLimits{ProviderID: "work"} }}},
		Grok:            []GrokProfileCollector{{ID: "work", Collector: func(*string, int64) ProviderLimits { grokCalls++; return ProviderLimits{ProviderID: "work"} }}},
		Allowed:         map[string]bool{"work": true},
		AllowedFamilies: map[string]bool{"claude": true},
		AllowedProfiles: map[string]map[string]bool{"claude": {"work": true}},
	}
	got := CollectAllProviderLimits(nil, 0, opts)
	if claudeCalls != 1 || grokCalls != 0 || len(got) != 1 {
		t.Fatalf("claude=%d grok=%d got=%v", claudeCalls, grokCalls, got)
	}
	if opts.AllowsFamily("grok") {
		t.Fatal("cross-family profile id bypassed the family allowlist")
	}
}

func TestConfiguredCollectionBoundDisablesExternalObservers(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[providers]\nenabled = [\"claude\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sources := ResolvedCollectionBound().routedActivitySources()
	if sources.ompPi || sources.openCode {
		t.Fatalf("bounded sources = %+v", sources)
	}
}

func TestBoundedCollectorsSkipOMPWindowPool(t *testing.T) {
	old := borrowWindows
	defer func() { borrowWindows = old }()
	calls := 0
	borrowWindows = func(string, string, string, int64) *ProviderLimits { calls++; return nil }
	dir := t.TempDir()
	_ = CollectClaudeLimits(0, CollectClaudeLimitsOptions{
		StatusLineCachePath: filepath.Join(dir, "missing-cache"),
		ClaudeJSONPath:      filepath.Join(dir, "missing-json"),
		SkipBorrowedWindows: true,
	})
	_ = collectCodexLimitsIn(filepath.Join(dir, "codex"), "codex", "Codex", 0, true)
	if calls != 0 {
		t.Fatalf("bounded collectors read OMP window pool %d times", calls)
	}
}

func limitsTestIDToken(t *testing.T, claims any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + ".test-signature"
}

func writeLimitsCodexAuth(t *testing.T, home, token string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"tokens": map[string]any{"id_token": token}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultCollectOptions_SingleClaudeAccountEmailSetting(t *testing.T) {
	for _, tt := range []struct {
		name    string
		enabled bool
	}{
		{"disabled", false},
		{"enabled", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pluginConfigDir := t.TempDir()
			t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginConfigDir)
			profileDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(profileDir, ".claude.json"),
				[]byte(`{"oauthAccount":{"emailAddress":"person@example.com"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			accountEmail := "false"
			if tt.enabled {
				accountEmail = "true"
			}
			config := "[ui]\naccount_email = " + accountEmail + "\n" +
				"[[claude.profiles]]\nid = \"personal\"\nlabel = \"personal\"\nconfig_dir = \"" + profileDir + "\"\n"
			if err := os.WriteFile(filepath.Join(pluginConfigDir, "config.toml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			got := DefaultCollectOptions().Claude[0].Collector(nil, 0)
			if tt.enabled {
				if got.GroupLabel != "Claude" || got.AccountLabel != "person@example.com" {
					t.Fatalf("enabled grouping = %q/%q", got.GroupLabel, got.AccountLabel)
				}
			} else if got.GroupLabel != "" || got.AccountLabel != "" {
				t.Fatalf("disabled grouping = %q/%q", got.GroupLabel, got.AccountLabel)
			}
		})
	}
}

func TestDefaultCollectOptions_SingleCodexAccountEmail(t *testing.T) {
	pluginConfigDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginConfigDir)
	profileHome := t.TempDir()
	writeLimitsCodexAuth(t, profileHome, limitsTestIDToken(t, map[string]any{"email": "person@example.com"}))
	config := "[ui]\naccount_email = true\n" +
		"[[codex.profiles]]\nid = \"personal\"\nlabel = \"personal\"\ncodex_home = \"" + profileHome + "\"\n"
	if err := os.WriteFile(filepath.Join(pluginConfigDir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	got := DefaultCollectOptions().Codex[0].Collector(nil, 0)
	if got.Label != "Codex" || got.GroupLabel != "" || got.AccountLabel != "person@example.com" {
		t.Fatalf("single Codex display metadata = %q/%q/%q", got.Label, got.GroupLabel, got.AccountLabel)
	}
}

func TestDefaultCollectOptions_CodexAccountEmailFallbacks(t *testing.T) {
	for _, tt := range []struct {
		name     string
		authBody string
	}{
		{"missing token", `{"tokens":{}}`},
		{"malformed token", `{"tokens":{"id_token":"not-a-jwt"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pluginConfigDir := t.TempDir()
			t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginConfigDir)
			profileHome := t.TempDir()
			if err := os.WriteFile(filepath.Join(profileHome, "auth.json"), []byte(tt.authBody), 0o600); err != nil {
				t.Fatal(err)
			}
			config := "[ui]\naccount_email = true\n" +
				"[[codex.profiles]]\nid = \"personal\"\nlabel = \"personal\"\ncodex_home = \"" + profileHome + "\"\n"
			if err := os.WriteFile(filepath.Join(pluginConfigDir, "config.toml"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			got := DefaultCollectOptions().Codex[0].Collector(nil, 0)
			if got.GroupLabel != "" || got.AccountLabel != "" || got.Label != "personal" {
				t.Fatalf("fallback changed current rendering metadata: %+v", got)
			}
		})
	}
}

func TestProviderAllowlistExcludingCodexNeverReadsAccountEmail(t *testing.T) {
	pluginConfigDir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", pluginConfigDir)
	config := "[ui]\naccount_email = true\n[providers]\nenabled = [\"claude\"]\n" +
		"[[codex.profiles]]\nid = \"personal\"\nlabel = \"personal\"\ncodex_home = \"/must-not-read\"\n"
	if err := os.WriteFile(filepath.Join(pluginConfigDir, "config.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	old := codexAccountEmailIn
	t.Cleanup(func() { codexAccountEmailIn = old })
	calls := 0
	codexAccountEmailIn = func(string) string {
		calls++
		return "should-not-appear@example.com"
	}
	opts := DefaultCollectOptions()
	_ = CollectAllProviderLimits(nil, 0, opts)
	if calls != 0 {
		t.Fatalf("excluded Codex account email reader called %d times", calls)
	}
}
