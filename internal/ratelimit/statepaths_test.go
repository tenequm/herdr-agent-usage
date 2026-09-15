package ratelimit

import (
	"path/filepath"
	"testing"

	"github.com/senna-lang/herdr-agent-usage/internal/pluginstate"
)

func TestConfiguredStateRootOwnsDefaultNotifyState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	restore := pluginstate.Configure(root)
	defer restore()
	t.Setenv("USAGEBAR_STATE_DIR", "")
	t.Setenv("USAGEBAR_PROVIDER_NOTIFY_PATH", "")
	if got := stateFilePathIn(""); got != filepath.Join(root, "rate-limit-state.json") {
		t.Fatalf("state path = %q", got)
	}
	if got := lockFilePathIn(""); got != filepath.Join(root, "rate-limit-state.lock") {
		t.Fatalf("lock path = %q", got)
	}
	if got := providerStateFilePath(); got != filepath.Join(root, "provider-limit-notify-state.json") {
		t.Fatalf("provider state path = %q", got)
	}
}
