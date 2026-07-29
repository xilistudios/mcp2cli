package util

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// FindToonCLI returns the command prefix for the toon CLI, or "" if not found.
func FindToonCLI() string {
	if _, err := exec.LookPath("toon"); err == nil {
		return "toon"
	}
	if _, err := exec.LookPath("npx"); err == nil {
		return "npx @toon-format/cli"
	}
	return ""
}

// ToonEncode encodes JSON using the toon CLI. Returns the encoded string and true on success.
func ToonEncode(jsonStr string) (string, bool) {
	cmd := FindToonCLI()
	if cmd == "" {
		return "", false
	}
	parts := strings.Fields(cmd)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, parts[0], parts[1:]...)
	c.Stdin = strings.NewReader(jsonStr)
	out, err := c.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}
