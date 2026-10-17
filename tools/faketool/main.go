// Command faketool stands in for claude, codex and copilot in the development sandbox:
// it prints how llmenu launched it instead of starting the real tool. Keys
// are masked.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	fmt.Printf("[sandbox] %s would start with:\n  args: %q\n", name, os.Args[1:])
	var env []string
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		switch {
		case k == "ANTHROPIC_AUTH_TOKEN" || k == "LLMENU_API_KEY" || k == "COPILOT_PROVIDER_API_KEY":
			env = append(env, k+"="+mask(v))
		case strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_DISABLE_") || strings.HasPrefix(k, "COPILOT_") || k == "DISABLE_AUTO_COMPACTION":
			env = append(env, kv)
		}
	}
	slices.Sort(env)
	for _, kv := range env {
		fmt.Println("  " + kv)
	}
}

func mask(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return v[:4] + "****"
}
