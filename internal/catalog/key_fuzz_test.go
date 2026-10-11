package catalog

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

var keyToken = regexp.MustCompile(`^([a-z]+|[0-9]+)$`)

// FuzzKey checks that Key yields sorted, non-empty tokens of only lowercase
// letters or only digits, without stopwords, and is stable when applied twice.
func FuzzKey(f *testing.F) {
	for _, seed := range []string{
		"bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0",
		"vertex_ai/claude-sonnet-4-5@20250929",
		"aws/claude-opus-latest",
		"hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8",
		"local/Qwen3.8-Flash-Next-non-thinking",
		"bedrock/zai.glm-5",
		"azure/gpt-5.6-sol",
		"", "/", "@", "a/", "-v1", "-12345678",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		k := Key(name, PickerStopwords)
		if k == "" {
			return
		}
		tokens := strings.Split(k, " ")
		for _, tok := range tokens {
			if !keyToken.MatchString(tok) {
				t.Fatalf("Key(%q) = %q: bad token %q", name, k, tok)
			}
			if slices.Contains(PickerStopwords, tok) {
				t.Fatalf("Key(%q) = %q: stopword %q", name, k, tok)
			}
		}
		if !slices.IsSorted(tokens) {
			t.Fatalf("Key(%q) = %q: not sorted", name, k)
		}
		if again := Key(k, PickerStopwords); again != k {
			t.Fatalf("Key(Key(%q)) = %q, want %q", name, again, k)
		}
	})
}

// FuzzShortTarget checks that ShortTarget only cuts: the result is part of
// the input's last path segment.
func FuzzShortTarget(f *testing.F) {
	for _, seed := range []string{
		"bedrock/eu.anthropic.claude-opus-4-6-v1",
		"bedrock/eu.anthropic.claude-sonnet-4-5-20250929-v1:0",
		"vertex_ai/claude-haiku-4-5@20251001",
		"hosted_vllm/Qwen/Qwen3.8-Flash-Next-FP8",
		"bedrock/zai.glm-5",
		"azure/gpt-6.1-sol",
		"", "/", "x/", "@",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, target string) {
		s := ShortTarget(target)
		if strings.Contains(s, "/") {
			t.Fatalf("ShortTarget(%q) = %q contains a slash", target, s)
		}
		if !strings.Contains(lastSegment(target), s) {
			t.Fatalf("ShortTarget(%q) = %q is not part of the input", target, s)
		}
	})
}
