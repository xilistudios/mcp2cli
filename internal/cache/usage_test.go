package cache

import (
	"os"
	"testing"

	"github.com/xilistudios/mcp2cli/internal/types"
)

// setCacheAndConfigDirs sets both MCP2CLI_CACHE_DIR and MCP2CLI_CONFIG_DIR
// to temp directories for test isolation.
func setCacheAndConfigDirs(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MCP2CLI_CACHE_DIR", dir)
	t.Setenv("MCP2CLI_CONFIG_DIR", dir)
}

// ---------------------------------------------------------------------------
// LoadUsage / SaveUsage
// ---------------------------------------------------------------------------

func TestLoadUsage_Empty(t *testing.T) {
	setCacheAndConfigDirs(t)
	u := LoadUsage()
	if len(u) != 0 {
		t.Fatalf("expected empty usage, got %v", u)
	}
}

func TestLoadSaveUsage_Roundtrip(t *testing.T) {
	setCacheAndConfigDirs(t)
	u := Usage{
		"abc123": {
			"my-tool": UsageEntry{Count: 5, LastUsed: "2026-01-01T00:00:00Z"},
		},
	}
	if err := SaveUsage(u); err != nil {
		t.Fatalf("SaveUsage: %v", err)
	}
	got := LoadUsage()
	if len(got) != 1 {
		t.Fatalf("expected 1 source, got %d", len(got))
	}
	entry := got["abc123"]["my-tool"]
	if entry.Count != 5 || entry.LastUsed != "2026-01-01T00:00:00Z" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestLoadUsage_CorruptJSON(t *testing.T) {
	setCacheAndConfigDirs(t)
	if err := os.WriteFile(UsageFile(), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := LoadUsage()
	if len(u) != 0 {
		t.Fatalf("expected empty usage for corrupt file, got %v", u)
	}
}

// ---------------------------------------------------------------------------
// RecordUsage
// ---------------------------------------------------------------------------

func TestRecordUsage_FirstCall(t *testing.T) {
	setCacheAndConfigDirs(t)
	if err := RecordUsage("src1", "tool-a"); err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}
	u := LoadUsage()
	entry := u["src1"]["tool-a"]
	if entry.Count != 1 {
		t.Fatalf("expected count 1, got %d", entry.Count)
	}
	if entry.LastUsed == "" {
		t.Fatal("expected non-empty last_used")
	}
}

func TestRecordUsage_Increment(t *testing.T) {
	setCacheAndConfigDirs(t)
	for i := 0; i < 3; i++ {
		if err := RecordUsage("src1", "tool-a"); err != nil {
			t.Fatalf("RecordUsage call %d: %v", i, err)
		}
	}
	u := LoadUsage()
	if u["src1"]["tool-a"].Count != 3 {
		t.Fatalf("expected count 3, got %d", u["src1"]["tool-a"].Count)
	}
}

func TestRecordUsage_MultipleTools(t *testing.T) {
	setCacheAndConfigDirs(t)
	RecordUsage("src1", "tool-a")
	RecordUsage("src1", "tool-b")
	RecordUsage("src1", "tool-a")
	u := LoadUsage()
	if u["src1"]["tool-a"].Count != 2 {
		t.Fatalf("tool-a count: expected 2, got %d", u["src1"]["tool-a"].Count)
	}
	if u["src1"]["tool-b"].Count != 1 {
		t.Fatalf("tool-b count: expected 1, got %d", u["src1"]["tool-b"].Count)
	}
}

func TestRecordUsage_MultipleSources(t *testing.T) {
	setCacheAndConfigDirs(t)
	RecordUsage("src1", "tool-a")
	RecordUsage("src2", "tool-a")
	u := LoadUsage()
	if u["src1"]["tool-a"].Count != 1 {
		t.Fatalf("src1 count: expected 1, got %d", u["src1"]["tool-a"].Count)
	}
	if u["src2"]["tool-a"].Count != 1 {
		t.Fatalf("src2 count: expected 1, got %d", u["src2"]["tool-a"].Count)
	}
}

func TestRecordUsage_PersistsAcrossLoad(t *testing.T) {
	setCacheAndConfigDirs(t)
	RecordUsage("src1", "tool-a")

	// Simulate a fresh session by loading again.
	u := LoadUsage()
	if u["src1"]["tool-a"].Count != 1 {
		t.Fatalf("expected persisted count 1, got %d", u["src1"]["tool-a"].Count)
	}
}

// ---------------------------------------------------------------------------
// SourceHashFor
// ---------------------------------------------------------------------------

func TestSourceHashFor_Deterministic(t *testing.T) {
	h1 := SourceHashFor("http://example.com")
	h2 := SourceHashFor("http://example.com")
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %q then %q", h1, h2)
	}
}

func TestSourceHashFor_DifferentSources(t *testing.T) {
	h1 := SourceHashFor("http://example.com")
	h2 := SourceHashFor("http://other.com")
	if h1 == h2 {
		t.Fatal("different sources should produce different hashes")
	}
}

func TestSourceHashFor_Length(t *testing.T) {
	h := SourceHashFor("anything")
	if len(h) != 16 {
		t.Fatalf("expected 16-char hex, got %d chars: %q", len(h), h)
	}
}

// ---------------------------------------------------------------------------
// SortCommands helpers
// ---------------------------------------------------------------------------

func makeCommands(names []string) []types.CommandDef {
	out := make([]types.CommandDef, len(names))
	for i, n := range names {
		out[i] = types.CommandDef{
			Name:     n,
			ToolName: n,
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// SortCommands
// ---------------------------------------------------------------------------

func TestSortCommands_DefaultPreservesOrder(t *testing.T) {
	setCacheAndConfigDirs(t)
	cmds := makeCommands([]string{"c", "a", "b"})
	result := SortCommands(cmds, "default", "src1")
	names := cmdNames(result)
	if names[0] != "c" || names[1] != "a" || names[2] != "b" {
		t.Fatalf("expected [c a b], got %v", names)
	}
}

func TestSortCommands_Alpha(t *testing.T) {
	setCacheAndConfigDirs(t)
	cmds := makeCommands([]string{"c", "a", "b"})
	result := SortCommands(cmds, "alpha", "src1")
	names := cmdNames(result)
	if names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Fatalf("expected [a b c], got %v", names)
	}
}

func TestSortCommands_UsageSort(t *testing.T) {
	setCacheAndConfigDirs(t)
	RecordUsage("src1", "b")
	RecordUsage("src1", "b")
	RecordUsage("src1", "b")
	RecordUsage("src1", "a")

	cmds := makeCommands([]string{"a", "b", "c"})
	result := SortCommands(cmds, "usage", "src1")
	names := cmdNames(result)
	if names[0] != "b" || names[1] != "a" || names[2] != "c" {
		t.Fatalf("expected [b a c], got %v", names)
	}
}

func TestSortCommands_RecentSort(t *testing.T) {
	setCacheAndConfigDirs(t)
	// Write usage data directly with distinct timestamps to avoid
	// wall-clock granularity issues (RFC3339 is second-precise).
	u := Usage{
		"src1": {
			"a": UsageEntry{Count: 1, LastUsed: "2026-01-01T00:00:01Z"},
			"c": UsageEntry{Count: 1, LastUsed: "2026-01-01T00:00:02Z"},
			"b": UsageEntry{Count: 1, LastUsed: "2026-01-01T00:00:03Z"}, // most recent
		},
	}
	if err := SaveUsage(u); err != nil {
		t.Fatalf("SaveUsage: %v", err)
	}

	cmds := makeCommands([]string{"a", "b", "c"})
	result := SortCommands(cmds, "recent", "src1")
	if result[0].Name != "b" {
		t.Fatalf("expected b first (most recent), got %s", result[0].Name)
	}
}

func TestSortCommands_UsageNoDataPreservesOrder(t *testing.T) {
	setCacheAndConfigDirs(t)
	cmds := makeCommands([]string{"c", "a", "b"})
	result := SortCommands(cmds, "usage", "src1")
	names := cmdNames(result)
	if names[0] != "c" || names[1] != "a" || names[2] != "b" {
		t.Fatalf("expected [c a b], got %v", names)
	}
}

func TestSortCommands_RecentNoDataPreservesOrder(t *testing.T) {
	setCacheAndConfigDirs(t)
	cmds := makeCommands([]string{"c", "a", "b"})
	result := SortCommands(cmds, "recent", "src1")
	names := cmdNames(result)
	if names[0] != "c" || names[1] != "a" || names[2] != "b" {
		t.Fatalf("expected [c a b], got %v", names)
	}
}

func TestSortCommands_DoesNotMutateInput(t *testing.T) {
	setCacheAndConfigDirs(t)
	cmds := makeCommands([]string{"c", "a", "b"})
	SortCommands(cmds, "alpha", "src1")
	if cmds[0].Name != "c" {
		t.Fatal("SortCommands must not mutate the input slice")
	}
}

// ---------------------------------------------------------------------------
// ResolveSortMode
// ---------------------------------------------------------------------------

func TestResolveSortMode_ExplicitOverrides(t *testing.T) {
	setCacheAndConfigDirs(t)
	if got := ResolveSortMode("alpha", "src1"); got != "alpha" {
		t.Fatalf("expected 'alpha', got %q", got)
	}
}

func TestResolveSortMode_DefaultNoData(t *testing.T) {
	setCacheAndConfigDirs(t)
	if got := ResolveSortMode("", "src1"); got != "default" {
		t.Fatalf("expected 'default', got %q", got)
	}
}

func TestResolveSortMode_UsageWithData(t *testing.T) {
	setCacheAndConfigDirs(t)
	RecordUsage("src1", "tool-a")
	if got := ResolveSortMode("", "src1"); got != "usage" {
		t.Fatalf("expected 'usage', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func cmdNames(cmds []types.CommandDef) []string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.Name
	}
	return names
}
