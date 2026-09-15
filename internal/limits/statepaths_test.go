package limits

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/senna-lang/herdr-agent-usage/internal/pluginstate"
)

func TestLegacyHistoryFileModeStaysReadable(t *testing.T) {
	restore := pluginstate.Configure("")
	defer restore()
	path := filepath.Join(t.TempDir(), "usage-history.json")
	t.Setenv("USAGEBAR_HISTORY_PATH", path)
	SaveUsageHistory(UsageHistory{"provider": {{T: 1, Used: 2}}})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("history mode = %o, want 644", info.Mode().Perm())
	}
}

func TestConfiguredStateRootOwnsGlobalLimitFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	restore := pluginstate.Configure(root)
	defer restore()
	t.Setenv("USAGEBAR_HISTORY_PATH", "")
	t.Setenv("USAGEBAR_OPENCODE_WEB_CACHE_PATH", "")
	t.Setenv("USAGEBAR_CLAUDE_LIMITS_PATH", "")
	if got := historyFilePath(); got != filepath.Join(root, "usage-history.json") {
		t.Fatalf("history path = %q", got)
	}
	if got := openCodeWebCachePath(); got != filepath.Join(root, "opencode-go-web.json") {
		t.Fatalf("OpenCode cache path = %q", got)
	}
	if got := ResolveClaudeLimitsCachePath(); got != filepath.Join(root, "claude", "claude", "claude-limits-latest.json") {
		t.Fatalf("Claude cache path = %q", got)
	}
}
