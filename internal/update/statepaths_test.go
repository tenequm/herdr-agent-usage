package update

import (
	"path/filepath"
	"testing"

	"github.com/senna-lang/herdr-agent-usage/internal/pluginstate"
)

func TestConfiguredStateRootOwnsHeartbeatAndWatchLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	restore := pluginstate.Configure(root)
	defer restore()
	t.Setenv("USAGEBAR_STATE_DIR", "")
	t.Setenv("USAGEBAR_PANE_HEARTBEAT_PATH", "")
	t.Setenv("USAGEBAR_WATCH_LOCK_PATH", "")
	if got := paneHeartbeatPath(); got != filepath.Join(root, heartbeatFileName) {
		t.Fatalf("heartbeat path = %q", got)
	}
	if got := watchLockPath(); got != filepath.Join(root, watchLockName) {
		t.Fatalf("watch lock path = %q", got)
	}
}
