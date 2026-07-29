package bake

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// NameRe validates baked tool names: must start with a lowercase letter,
// followed by lowercase letters, digits, or hyphens.
var NameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Create saves a new baked tool. Returns an error if the name is invalid
// or already exists (unless force is true).
func Create(name string, c Config, force bool) error {
	if !NameRe.MatchString(name) {
		return fmt.Errorf("invalid name %q — must match [a-z][a-z0-9-]*", name)
	}
	all, err := LoadAll()
	if err != nil {
		return err
	}
	if _, exists := all[name]; exists && !force {
		return fmt.Errorf("%q already exists. Use --force to overwrite", name)
	}
	all[name] = c
	return SaveAll(all)
}

// Remove deletes a baked tool by name. Also removes any installed wrapper
// script from ~/.local/bin (best-effort).
func Remove(name string) error {
	all, err := LoadAll()
	if err != nil {
		return err
	}
	if _, ok := all[name]; !ok {
		return fmt.Errorf("no baked tool named %q", name)
	}
	delete(all, name)
	if err := SaveAll(all); err != nil {
		return err
	}
	// Best-effort remove installed wrapper.
	home, err := os.UserHomeDir()
	if err == nil {
		wrapper := filepath.Join(home, ".local", "bin", name)
		os.Remove(wrapper) // ignore error
	}
	return nil
}

// UpdateOptions holds optional fields for Update. Only non-nil fields
// are applied.
type UpdateOptions struct {
	CacheTTL    *int
	Include     *[]string
	Exclude     *[]string
	Methods     *[]string
	Description *string
	BaseURL     *string
	Transport   *string
}

// Update applies partial modifications to an existing baked tool.
func Update(name string, o UpdateOptions) error {
	all, err := LoadAll()
	if err != nil {
		return err
	}
	cfg, ok := all[name]
	if !ok {
		return fmt.Errorf("no baked tool named %q", name)
	}
	if o.CacheTTL != nil {
		cfg.CacheTTL = *o.CacheTTL
	}
	if o.Include != nil {
		cfg.Include = *o.Include
	}
	if o.Exclude != nil {
		cfg.Exclude = *o.Exclude
	}
	if o.Methods != nil {
		upper := make([]string, len(*o.Methods))
		for i, m := range *o.Methods {
			upper[i] = strings.ToUpper(m)
		}
		cfg.Methods = upper
	}
	if o.Description != nil {
		cfg.Description = *o.Description
	}
	if o.BaseURL != nil {
		cfg.BaseURL = *o.BaseURL
	}
	if o.Transport != nil {
		cfg.Transport = *o.Transport
	}
	all[name] = cfg
	return SaveAll(all)
}

// List prints a formatted table of all baked tools to w.
func List(w io.Writer) error {
	all, err := LoadAll()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(w, "No baked tools.")
		return nil
	}
	fmt.Fprintf(w, "%-20s %-10s %-50s\n", "Name", "Type", "Source")
	fmt.Fprintln(w, strings.Repeat("-", 80))
	for _, name := range sortedKeys(all) {
		cfg := all[name]
		src := cfg.SourceType
		source := cfg.Source
		if len(source) > 48 {
			source = source[:45] + "..."
		}
		fmt.Fprintf(w, "%-20s %-10s %-50s\n", name, src, source)
	}
	return nil
}

// Show returns a display map for a baked tool with auth_header secrets masked.
func Show(name string) (map[string]any, error) {
	cfg, ok, err := Load(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no baked tool named %q", name)
	}
	// Round-trip through JSON to get a generic map.
	raw, _ := json.Marshal(cfg)
	var display map[string]any
	json.Unmarshal(raw, &display)

	// Mask auth header values.
	if headers, ok := display["auth_headers"].([]any); ok {
		masked := make([]any, len(headers))
		for i, h := range headers {
			pair, ok := h.([]any)
			if !ok || len(pair) < 2 {
				masked[i] = h
				continue
			}
			val, _ := pair[1].(string)
			if strings.HasPrefix(val, "env:") || strings.HasPrefix(val, "file:") {
				masked[i] = h
			} else if len(val) > 4 {
				masked[i] = []any{pair[0], val[:4] + "****"}
			} else {
				masked[i] = []any{pair[0], "****"}
			}
		}
		display["auth_headers"] = masked
	}

	return display, nil
}

// Install creates an executable wrapper script in the given directory.
// If dir is empty, defaults to ~/.local/bin. Returns the wrapper path.
func Install(name, dir string) (string, error) {
	cfg, ok, err := Load(name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no baked tool named %q", name)
	}

	binDir := dir
	if binDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		binDir = filepath.Join(home, ".local", "bin")
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("creating bin dir: %w", err)
	}

	mcp2cliBin, err := exec.LookPath("mcp2cli")
	if err != nil {
		mcp2cliBin = "mcp2cli"
	}

	wrapper := filepath.Join(binDir, name)
	content := "#!/bin/sh\nexec " + shlexQuote(mcp2cliBin) + " @" + name + " \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("writing wrapper: %w", err)
	}

	// Ensure executable.
	if err := os.Chmod(wrapper, 0o755); err != nil {
		return "", fmt.Errorf("chmod wrapper: %w", err)
	}

	_ = cfg // used for validation above
	return wrapper, nil
}

// shlexQuote quotes a shell argument. If the string contains only safe
// characters it is returned as-is; otherwise it is wrapped in single quotes
// with embedded single quotes escaped.
func shlexQuote(s string) string {
	if regexp.MustCompile(`^[A-Za-z0-9_/\.\-]+$`).MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]Config) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort for small maps.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
