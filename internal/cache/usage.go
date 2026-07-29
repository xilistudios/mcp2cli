package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"time"

	"github.com/xilistudios/mcp2cli/internal/types"
)

// UsageEntry records how many times a tool has been called and when it
// was last used.
type UsageEntry struct {
	Count    int    `json:"count"`
	LastUsed string `json:"last_used"`
}

// Usage maps source_hash → tool_name → UsageEntry.
type Usage map[string]map[string]UsageEntry

// LoadUsage reads the usage file from disk. It returns an empty Usage on
// any failure (missing file, corrupt JSON, read error).
func LoadUsage() Usage {
	raw, err := os.ReadFile(UsageFile())
	if err != nil {
		return Usage{}
	}
	var u Usage
	if err := json.Unmarshal(raw, &u); err != nil {
		return Usage{}
	}
	return u
}

// SaveUsage writes the usage data to disk as indented JSON. It creates
// the cache directory if needed.
func SaveUsage(u Usage) error {
	dir := CacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(UsageFile(), raw, 0o644)
}

// RecordUsage increments the call count and updates last_used for the
// given (sourceHash, toolName) pair. It is a load-mutate-save operation.
func RecordUsage(sourceHash, toolName string) error {
	u := LoadUsage()
	if u[sourceHash] == nil {
		u[sourceHash] = map[string]UsageEntry{}
	}
	entry := u[sourceHash][toolName]
	entry.Count++
	entry.LastUsed = time.Now().UTC().Format(time.RFC3339)
	u[sourceHash][toolName] = entry
	return SaveUsage(u)
}

// SourceHashFor derives a stable 16-hex-char hash key from a source
// URL or command string.
func SourceHashFor(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])[:16]
}

// SortCommands returns a new slice of CommandDef sorted according to
// sortMode. The input slice is never mutated.
//
// Supported modes:
//
//	"default" – return a copy in the original order
//	"alpha"   – alphabetical by Name
//	"usage"   – most-used first (by count descending)
//	"recent"  – most-recently-used first (by last_used string descending)
//
// For "usage" and "recent" modes, the lookup key for each command is the
// first non-empty value among ToolName, GraphQLFieldName, or Name. If
// no usage data exists for sourceHash the original order is preserved.
func SortCommands(commands []types.CommandDef, sortMode, sourceHash string) []types.CommandDef {
	// Always work on a copy.
	out := make([]types.CommandDef, len(commands))
	copy(out, commands)

	switch sortMode {
	case "default":
		return out

	case "alpha":
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Name < out[j].Name
		})
		return out

	case "usage":
		usage := LoadUsage()
		bucket := usage[sourceHash]
		if len(bucket) == 0 {
			return out
		}
		sort.SliceStable(out, func(i, j int) bool {
			ci := bucket[usageKey(out[i])].Count
			cj := bucket[usageKey(out[j])].Count
			return ci > cj
		})
		return out

	case "recent":
		usage := LoadUsage()
		bucket := usage[sourceHash]
		if len(bucket) == 0 {
			return out
		}
		sort.SliceStable(out, func(i, j int) bool {
			li := bucket[usageKey(out[i])].LastUsed
			lj := bucket[usageKey(out[j])].LastUsed
			return li > lj
		})
		return out

	default:
		return out
	}
}

// usageKey returns the first non-empty lookup key for a command
// (ToolName → GraphQLFieldName → Name).
func usageKey(c types.CommandDef) string {
	if c.ToolName != "" {
		return c.ToolName
	}
	if c.GraphQLFieldName != "" {
		return c.GraphQLFieldName
	}
	return c.Name
}

// ResolveSortMode determines the effective sort mode. If explicit is
// non-empty it is returned directly. Otherwise, "usage" is returned when
// usage data exists for sourceHash, "default" otherwise.
func ResolveSortMode(explicit, sourceHash string) string {
	if explicit != "" {
		return explicit
	}
	u := LoadUsage()
	if len(u[sourceHash]) > 0 {
		return "usage"
	}
	return "default"
}
