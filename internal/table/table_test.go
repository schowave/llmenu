package table

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/schowave/llmenu/internal/catalog"
)

func f(v float64) *float64 { return &v }

var (
	opus   = catalog.Model{Name: "aws/claude-opus-5-5", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000, InputPrice: 5, OutputPrice: 25}
	alias  = catalog.Model{Name: "aws/claude-opus-latest", Target: "bedrock/eu.anthropic.claude-opus-5-5", ContextTokens: 1_000_000, InputPrice: 5, OutputPrice: 25}
	gpt    = catalog.Model{Name: "azure/gpt-5.6-sol", Target: "azure/gpt-5.6-sol", ContextTokens: 400_000, InputPrice: 1.25, OutputPrice: 10}
	tiered = catalog.Model{Name: "aws/claude-haiku-5-5", ContextTokens: 1_000_000, InputPrice: 0.11, OutputPrice: 0.55,
		Tiers: []catalog.Tier{{AboveTokens: 100_000, InputPrice: 0.55, OutputPrice: 2.75}}}
	benchd = catalog.Model{Name: "x/ä", Target: "x/ä", ContextTokens: 8_000, Bench: &catalog.Bench{Intelligence: f(57.64), Coding: f(80)}}
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name   string
		models []catalog.Model
		opt    Options
		header string
		rows   []string
	}{
		{
			name:   "plain",
			models: []catalog.Model{opus, gpt},
			header: "MODEL                CONTEXT      $ IN    $ OUT",
			rows: []string{
				"aws/claude-opus-5-5       1M      5.00    25.00",
				"azure/gpt-5.6-sol       400k      1.25    10.00",
			},
		},
		{
			name:   "alias without target",
			models: []catalog.Model{alias},
			header: "MODEL                   CONTEXT      $ IN    $ OUT",
			rows:   []string{"aws/claude-opus-latest       1M      5.00    25.00"},
		},
		{
			name:   "alias with target",
			models: []catalog.Model{alias, opus},
			opt:    Options{ShowAliasTarget: true},
			header: "MODEL                                      CONTEXT      $ IN    $ OUT",
			rows: []string{
				"aws/claude-opus-latest -> claude-opus-5-5       1M      5.00    25.00",
				"aws/claude-opus-5-5                             1M      5.00    25.00",
			},
		},
		{
			name:   "mark column",
			models: []catalog.Model{opus, gpt},
			opt: Options{Mark: func(m catalog.Model) string {
				if m.Name == gpt.Name {
					return "!"
				}
				return ""
			}},
			header: "  MODEL                CONTEXT      $ IN    $ OUT",
			rows: []string{
				"  aws/claude-opus-5-5       1M      5.00    25.00",
				"! azure/gpt-5.6-sol       400k      1.25    10.00",
			},
		},
		{
			name:   "no mark column without marks",
			models: []catalog.Model{opus},
			opt:    Options{Mark: func(catalog.Model) string { return "" }},
			header: "MODEL                CONTEXT      $ IN    $ OUT",
			rows:   []string{"aws/claude-opus-5-5       1M      5.00    25.00"},
		},
		{
			name:   "benchmarks, multi-byte name",
			models: []catalog.Model{benchd, gpt},
			header: "MODEL              CONTEXT      $ IN    $ OUT   INTEL    TB4",
			rows: []string{
				"x/ä                     8k      free     free    57.6      -",
				"azure/gpt-5.6-sol     400k      1.25    10.00       -      -",
			},
		},
		{
			name:   "coding column when sorted by it",
			models: []catalog.Model{benchd},
			opt:    Options{Code: true},
			header: "MODEL  CONTEXT      $ IN    $ OUT   INTEL    TB4   CODE",
			rows:   []string{"x/ä         8k      free     free    57.6      -   80.0"},
		},
		{
			name:   "unknown price, tier mark",
			models: []catalog.Model{{Name: "x/none", NoPrice: true}, tiered},
			header: "MODEL                 CONTEXT      $ IN    $ OUT",
			rows: []string{
				"x/none                      -         -        -",
				"aws/claude-haiku-5-5       1M      0.11     0.55*",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, rows := Format(tt.models, tt.opt)
			if header != tt.header {
				t.Errorf("header\n%q\nwant\n%q", header, tt.header)
			}
			if strings.Join(rows, "\n") != strings.Join(tt.rows, "\n") {
				t.Errorf("rows\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(tt.rows, "\n"))
			}
			// Columns line up: a row is as wide as the header, or one wider
			// for a tier mark after the last price.
			for _, r := range rows {
				if w, hw := utf8.RuneCountInString(r), utf8.RuneCountInString(header); w != hw && (w != hw+1 || !strings.HasSuffix(r, "*")) {
					t.Errorf("row %q not as wide as the header", r)
				}
			}
		})
	}
}

func TestFormatEmpty(t *testing.T) {
	header, rows := Format(nil, Options{})
	if header != "MODEL  CONTEXT      $ IN    $ OUT" || len(rows) != 0 {
		t.Errorf("Format(nil) = %q, %q", header, rows)
	}
}

func TestTokens(t *testing.T) {
	for n, want := range map[int]string{
		0: "-", -1: "-", 999: "0k", 200_000: "200k", 922_000: "922k", 999_999: "999k",
		1_000_000: "1M", 1_048_576: "1.05M", 2_000_000: "2M", 1_500_000: "1.5M",
	} {
		if got := Tokens(n); got != want {
			t.Errorf("Tokens(%d) = %q, want %q", n, got, want)
		}
	}
}
