package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ParseCommandArgs parses tool args for a single command.
// It registers a --stdin bool flag when cmd.HasBody, plus one flag per
// param (deduped by flag name).
// Returns: values map keyed by param.Name (kebab) for flags that were
// explicitly set; hasStdin bool.
func ParseCommandArgs(cmd types.CommandDef, args []string) (values map[string]any, hasStdin bool, err error) {
	fs := flag.NewFlagSet(cmd.Name, flag.ContinueOnError)
	fs.SetOutput(nil) // suppress default error output

	var stdin bool
	if cmd.HasBody {
		fs.BoolVar(&stdin, "stdin", false, "Read JSON body/arguments from stdin")
	}

	strPtrs := map[string]*string{}
	intPtrs := map[string]*int{}
	floatPtrs := map[string]*float64{}
	boolPtrs := map[string]*bool{}
	paramByName := map[string]types.ParamDef{}

	seen := map[string]bool{}
	for _, p := range cmd.Params {
		flagName := p.Name
		if seen[flagName] {
			continue
		}
		seen[flagName] = true
		paramByName[flagName] = p

		switch p.Type {
		case types.TypeBoolean:
			boolPtrs[flagName] = fs.Bool(flagName, false, p.Description)
		case types.TypeInt:
			intPtrs[flagName] = fs.Int(flagName, 0, p.Description)
		case types.TypeFloat:
			floatPtrs[flagName] = fs.Float64(flagName, 0, p.Description)
		default:
			strPtrs[flagName] = fs.String(flagName, "", p.Description)
		}
	}

	if err := fs.Parse(args); err != nil {
		return nil, false, err
	}

	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		set[f.Name] = true
	})

	values = map[string]any{}
	for name, p := range paramByName {
		if !set[name] {
			continue
		}
		switch p.Type {
		case types.TypeBoolean:
			values[name] = *boolPtrs[name]
		case types.TypeInt:
			values[name] = util.CoerceValue(*intPtrs[name], p.Schema)
		case types.TypeFloat:
			values[name] = util.CoerceValue(*floatPtrs[name], p.Schema)
		default:
			values[name] = util.CoerceValue(*strPtrs[name], p.Schema)
		}
	}

	// Validation: required params.
	for _, p := range cmd.Params {
		if p.Required && p.Type != types.TypeBoolean &&
			(p.Location == "path" || p.Location == "query" || p.Location == "header") &&
			!set[p.Name] {
			return nil, false, fmt.Errorf("the following arguments are required: --%s", p.Name)
		}
	}

	// Validation: choices.
	for name, p := range paramByName {
		if p.Choices != nil && set[name] {
			val := fmt.Sprint(values[name])
			if !contains(p.Choices, val) {
				return nil, false, fmt.Errorf("argument --%s: invalid choice: %q (choose from %s)", name, val, strings.Join(p.Choices, ", "))
			}
		}
	}

	return values, stdin, nil
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
