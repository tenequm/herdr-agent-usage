/**
 * Tests for the startup republish of sidebar tokens across open agent panes.
 *
 * After a Herdr restart the server-side tokens are gone. This path must
 * visit every listed pane once, never force-write (so live-token dedupe
 * still applies on handoff), and write nothing when the pane query failed.
 */
package update

import (
	"reflect"
	"testing"

	"github.com/senna-lang/herdr-agent-usage/internal/herdrcli"
)

func TestRepublishOpenAgentPanesWith_UpdatesEachListedPane(t *testing.T) {
	var got []string
	var forced []bool
	republishOpenAgentPanesWith(
		func() ([]herdrcli.OpenAgentPane, bool) {
			return []herdrcli.OpenAgentPane{
				{PaneID: "w1:p1"},
				{PaneID: "w1:p2"},
			}, true
		},
		func(paneID string, force bool) {
			got = append(got, paneID)
			forced = append(forced, force)
		},
	)
	want := []string{"w1:p1", "w1:p2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("updated %v, want %v", got, want)
	}
	for i, force := range forced {
		if force {
			t.Fatalf("pane %s: startup must not force-write tokens", got[i])
		}
	}
}

func TestRepublishOpenAgentPanesWith_FailedListWritesNothing(t *testing.T) {
	called := false
	republishOpenAgentPanesWith(
		func() ([]herdrcli.OpenAgentPane, bool) {
			return []herdrcli.OpenAgentPane{{PaneID: "w1:p1"}}, false
		},
		func(string, bool) { called = true },
	)
	if called {
		t.Fatal("a failed pane query must not republish tokens")
	}
}

func TestRepublishOpenAgentPanesWith_EmptyListWritesNothing(t *testing.T) {
	called := false
	republishOpenAgentPanesWith(
		func() ([]herdrcli.OpenAgentPane, bool) {
			return nil, true
		},
		func(string, bool) { called = true },
	)
	if called {
		t.Fatal("no open agent panes must not republish tokens")
	}
}

func TestRepublishOpenAgentPanesWith_SkipsEmptyPaneID(t *testing.T) {
	var got []string
	republishOpenAgentPanesWith(
		func() ([]herdrcli.OpenAgentPane, bool) {
			return []herdrcli.OpenAgentPane{
				{PaneID: ""},
				{PaneID: "w1:p3"},
			}, true
		},
		func(paneID string, force bool) {
			got = append(got, paneID)
		},
	)
	if !reflect.DeepEqual(got, []string{"w1:p3"}) {
		t.Fatalf("updated %v, want [w1:p3]", got)
	}
}

func TestClearOpenAgentPaneMetadataWithClearsOnlyPresentOwnedTokens(t *testing.T) {
	var reads []string
	var clears []string
	clearOpenAgentPaneMetadataWith(
		func() ([]herdrcli.OpenAgentPane, bool) {
			return []herdrcli.OpenAgentPane{{PaneID: "p1"}, {PaneID: ""}, {PaneID: "p2"}}, true
		},
		func(paneID string) (herdrcli.PaneInfo, bool) {
			reads = append(reads, paneID)
			return herdrcli.PaneInfo{Tokens: map[string]string{
				"provider": "claude",
				"context":  "50%",
				"foreign":  "keep",
			}}, true
		},
		metadataTokenWriter{clear: func(paneID, source, name string) bool {
			if source != herdrcli.Source {
				t.Fatalf("source=%q", source)
			}
			clears = append(clears, paneID+":"+name)
			return true
		}},
	)
	if !reflect.DeepEqual(reads, []string{"p1", "p2"}) {
		t.Fatalf("metadata reads %v", reads)
	}
	want := []string{"p1:provider", "p1:context", "p2:provider", "p2:context"}
	if !reflect.DeepEqual(clears, want) {
		t.Fatalf("cleared %v, want %v", clears, want)
	}
}

func TestClearOpenAgentPaneMetadataWithNoOwnedTokensWritesNothing(t *testing.T) {
	writes := 0
	clearOpenAgentPaneMetadataWith(
		func() ([]herdrcli.OpenAgentPane, bool) {
			return []herdrcli.OpenAgentPane{{PaneID: "p1"}}, true
		},
		func(string) (herdrcli.PaneInfo, bool) {
			return herdrcli.PaneInfo{Tokens: map[string]string{"foreign": "keep"}}, true
		},
		metadataTokenWriter{clear: func(string, string, string) bool {
			writes++
			return true
		}},
	)
	if writes != 0 {
		t.Fatalf("pane with no owned tokens made %d clears", writes)
	}
}

func TestClearOpenAgentPaneMetadataWithUnreadablePaneClearsAllOwnedTokens(t *testing.T) {
	var got []string
	clearOpenAgentPaneMetadataWith(
		func() ([]herdrcli.OpenAgentPane, bool) {
			return []herdrcli.OpenAgentPane{{PaneID: "p1"}}, true
		},
		func(string) (herdrcli.PaneInfo, bool) { return herdrcli.PaneInfo{}, false },
		metadataTokenWriter{clear: func(paneID, source, name string) bool {
			if paneID != "p1" || source != herdrcli.Source {
				t.Fatalf("target=%q source=%q", paneID, source)
			}
			got = append(got, name)
			return true
		}},
	)
	if !reflect.DeepEqual(got, ownedMetadataTokenNames) {
		t.Fatalf("cleared %v, want %v", got, ownedMetadataTokenNames)
	}
}

func TestClearPaneMetadataWithClearsOnlyPresentOwnedTokens(t *testing.T) {
	var got []string
	clearPaneMetadataWith(metadataTokenWriter{clear: func(paneID, source, name string) bool {
		if paneID != "p1" || source != herdrcli.Source {
			t.Fatalf("target=%q source=%q", paneID, source)
		}
		got = append(got, name)
		return true
	}}, map[string]string{"limit": "old", "foreign": "keep"}, "p1")
	if !reflect.DeepEqual(got, []string{"limit"}) {
		t.Fatalf("cleared %v, want [limit]", got)
	}

	got = nil
	clearPaneMetadataWith(metadataTokenWriter{clear: func(string, string, string) bool {
		got = append(got, "unexpected")
		return true
	}}, map[string]string{}, "p1")
	if len(got) != 0 {
		t.Fatalf("empty current tokens produced clears: %v", got)
	}
}
