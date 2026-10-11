// Command llmenu selects models from a LiteLLM gateway and wires them into
// coding tools (see docs/ADR).
package main

import (
	"os"

	"github.com/schowave/llmenu/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
