/**
 * Republishes sidebar tokens for every open agent pane after Herdr
 * restores the session (and again after live handoff).
 *
 * Server-owned metadata tokens do not survive a cold restart. The event
 * path only sees HERDR_PANE_ID for one pane, and PublishCollectedLimits
 * writes $limit only. Startup therefore reuses RunUpdateForPane so $title,
 * $provider, $limit, $cache, and $context — including the post-compaction label
 * and pay-as-you-go burn — come back in one pass. force is false so the
 * live-token dedupe still applies when handoff leaves tokens already set.
 */
package update

import "github.com/senna-lang/herdr-agent-usage/internal/herdrcli"

var ownedMetadataTokenNames = []string{"title", "provider", "limit", "context", "cache", "cache_high", "cache_mid", "cache_low"}

// ClearPaneMetadata removes the plugin-owned tokens currently present on one
// pane, without issuing clears for absent tokens.
func ClearPaneMetadata(paneID string, current map[string]string) {
	clearPaneMetadataWith(herdrMetadataTokenWriter, current, paneID)
}

func clearPaneMetadataWith(writer metadataTokenWriter, current map[string]string, paneID string) {
	if paneID == "" {
		return
	}
	for _, name := range ownedMetadataTokenNames {
		if _, present := current[name]; present {
			writer.clear(paneID, herdrcli.Source, name)
		}
	}
}

func clearAllPaneMetadataWith(writer metadataTokenWriter, paneID string) {
	if paneID == "" {
		return
	}
	for _, name := range ownedMetadataTokenNames {
		writer.clear(paneID, herdrcli.Source, name)
	}
}

// ClearOpenAgentPaneMetadata removes stale plugin tokens from every open pane.
// It reads each pane once and avoids issuing clears for tokens that are absent.
// An unreadable pane falls back to clearing every owned token because its
// server-side state is unknown.
func ClearOpenAgentPaneMetadata() {
	clearOpenAgentPaneMetadataWith(herdrcli.ListOpenAgentPanesOK, herdrcli.GetPaneInfoOK, herdrMetadataTokenWriter)
}

func clearOpenAgentPaneMetadataWith(
	list func() ([]herdrcli.OpenAgentPane, bool),
	get func(string) (herdrcli.PaneInfo, bool),
	writer metadataTokenWriter,
) {
	panes, ok := list()
	if !ok {
		return
	}
	for _, pane := range panes {
		if pane.PaneID == "" {
			continue
		}
		current, ok := get(pane.PaneID)
		if !ok {
			clearAllPaneMetadataWith(writer, pane.PaneID)
			continue
		}
		clearPaneMetadataWith(writer, current.Tokens, pane.PaneID)
	}
}

// RepublishOpenAgentPanes restores sidebar tokens for every open agent pane.
func RepublishOpenAgentPanes() {
	republishOpenAgentPanesWith(herdrcli.ListOpenAgentPanesOK, RunUpdateForPane)
}

func republishOpenAgentPanesWith(
	list func() ([]herdrcli.OpenAgentPane, bool),
	updatePane func(paneID string, force bool),
) {
	panes, ok := list()
	if !ok {
		return
	}
	for _, pane := range panes {
		if pane.PaneID == "" {
			continue
		}
		updatePane(pane.PaneID, false)
	}
}
