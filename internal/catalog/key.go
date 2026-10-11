package catalog

import (
	"regexp"
	"slices"
	"strings"
)

// PickerStopwords are the tokens that do not count for selection: region,
// provider, thinking variants and -latest. Variants like -high or -max
// deliberately stay distinguishable; benchmark matching uses a longer list
// (see `llmenu bench`).
var PickerStopwords = []string{"eu", "us", "global", "anthropic", "zai", "thinking", "non", "latest"}

var (
	reVersion = regexp.MustCompile(`-v[0-9]+(:[0-9]+)?$`)
	reDate    = regexp.MustCompile(`-[0-9]{8}`)
	reDateEnd = regexp.MustCompile(`-[0-9]{8}$`)
	reFP8     = regexp.MustCompile(`(?i)-fp8$`)
	reToken   = regexp.MustCompile(`[a-z]+|[0-9]+`)
	reRegion  = regexp.MustCompile(`^(eu|us|global)\.anthropic\.`)
)

// Key normalizes a model name to a set of tokens, so the same model looks
// identical across providers and spellings:
// "bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0" and
// "gcp/claude-sonnet-4-5@20250929" both yield "4 5 claude sonnet".
func Key(name string, stop []string) string {
	s := lastSegment(strings.ToLower(name))
	s, _, _ = strings.Cut(s, "@")
	s = reVersion.ReplaceAllString(s, "")
	s = reDate.ReplaceAllString(s, "")
	s = reFP8.ReplaceAllString(s, "")

	var tokens []string
	for _, t := range reToken.FindAllString(s, -1) {
		if !slices.Contains(stop, t) {
			tokens = append(tokens, t)
		}
	}
	slices.Sort(tokens)
	return strings.Join(tokens, " ")
}

// ShortTarget shortens a target model for display:
// "bedrock/eu.anthropic.claude-opus-4-6-v1" becomes "claude-opus-4-6".
func ShortTarget(target string) string {
	s := lastSegment(target)
	s = reRegion.ReplaceAllString(s, "")
	s = strings.TrimPrefix(s, "zai.")
	s, _, _ = strings.Cut(s, "@")
	s = reVersion.ReplaceAllString(s, "")
	s = reDateEnd.ReplaceAllString(s, "")
	return reFP8.ReplaceAllString(s, "")
}

func lastSegment(s string) string {
	return s[strings.LastIndex(s, "/")+1:]
}
