package util

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadStdinJSONFrom reads all data from r, parses it as JSON, and returns the result.
func ReadStdinJSONFrom(r io.Reader, context string) (any, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read stdin for %s: %w", context, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("--stdin expects JSON for %s, but stdin was empty.", context)
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		msg := fmt.Sprintf("invalid JSON on stdin for %s: %v", context, err)
		if se, ok := err.(*json.SyntaxError); ok {
			msg = fmt.Sprintf("invalid JSON on stdin for %s: syntax error at byte offset %d: %v", context, se.Offset, err)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return parsed, nil
}

// ReadStdinJSON reads JSON from os.Stdin.
func ReadStdinJSON(context string) (any, error) {
	return ReadStdinJSONFrom(os.Stdin, context)
}
