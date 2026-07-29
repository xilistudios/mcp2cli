package cli

import "strings"

// SplitAtSubcommand splits argv into (globalArgs, toolArgs) at the
// subcommand boundary. valueOpts are global options that consume a
// following value; boolOpts are global boolean flags (no value).
func SplitAtSubcommand(argv []string, valueOpts, boolOpts map[string]bool) (global, tool []string) {
	i := 0
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			return argv[:i], argv[i+1:]
		}
		if strings.HasPrefix(arg, "-") {
			if strings.HasPrefix(arg, "--") && strings.Contains(arg, "=") {
				i++
			} else if valueOpts[arg] {
				i += 2
			} else if boolOpts[arg] {
				i++
			} else {
				i++
			}
		} else {
			return argv[:i], argv[i:]
		}
	}
	return argv, nil
}
