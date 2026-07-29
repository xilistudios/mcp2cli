// mcp2cli — turn any MCP server, OpenAPI spec, or GraphQL endpoint into a CLI.
package main

import (
	"fmt"
	"os"

	"github.com/xilistudios/mcp2cli/internal/cli"
	"github.com/xilistudios/mcp2cli/internal/util"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(util.Err, "Error:", err)
		os.Exit(1)
	}
}
