package util

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Out is the standard output writer (overridable for testing).
var Out io.Writer = os.Stdout

// Err is the standard error writer (overridable for testing).
var Err io.Writer = os.Stderr

// StdoutIsTTY returns true if stdout is a character device (terminal).
// Overridable for testing.
var StdoutIsTTY = func() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ApplyHead truncates a slice to at most n elements. Non-slice values pass through.
func ApplyHead(data any, n int) any {
	if arr, ok := data.([]any); ok {
		if n < len(arr) {
			return arr[:n]
		}
		return arr
	}
	return data
}

// EmitJSON marshals data to JSON and writes it to Out.
// If pretty is true or StdoutIsTTY(), it uses indented output.
func EmitJSON(data any, pretty bool) {
	var b []byte
	var err error
	if pretty || StdoutIsTTY() {
		b, err = json.MarshalIndent(data, "", "  ")
	} else {
		b, err = json.Marshal(data)
	}
	if err != nil {
		fmt.Fprintf(Err, "Error marshaling JSON: %v\n", err)
		return
	}
	Out.Write(b)
	Out.Write([]byte("\n"))
}

// OutputOptions controls how OutputResult formats and emits data.
type OutputOptions struct {
	Pretty     bool
	Raw        bool
	Toon       bool
	Head       *int
	JSONOutput bool
}

// OutputResult emits data according to the given options.
func OutputResult(data any, opts OutputOptions) {
	// JSON output mode: always emit as JSON
	if opts.JSONOutput {
		if s, ok := data.(string); ok {
			var parsed any
			if json.Unmarshal([]byte(s), &parsed) == nil {
				data = parsed
			}
		}
		if opts.Head != nil {
			data = ApplyHead(data, *opts.Head)
		}
		EmitJSON(data, opts.Pretty)
		return
	}

	// Raw mode: print as-is
	if opts.Raw {
		if s, ok := data.(string); ok {
			fmt.Fprintln(Out, s)
		} else {
			b, _ := json.Marshal(data)
			fmt.Fprintln(Out, string(b))
		}
		return
	}

	// Default mode: try to parse JSON strings, then emit
	if s, ok := data.(string); ok {
		var parsed any
		if json.Unmarshal([]byte(s), &parsed) == nil {
			data = parsed
		} else {
			fmt.Fprintln(Out, s)
			return
		}
	}

	if opts.Head != nil {
		data = ApplyHead(data, *opts.Head)
	}

	if opts.Toon {
		b, _ := json.Marshal(data)
		if enc, ok := ToonEncode(string(b)); ok {
			fmt.Fprint(Out, enc)
			return
		}
		fmt.Fprintln(Err, "Warning: --toon requires the TOON CLI (@toon-format/cli). Install with: npm install -g @toon-format/cli")
	}

	EmitJSON(data, opts.Pretty)
}
