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
			header: "MODEL                CONTEXT      $ IN     $ OUT",
			rows: []string{
				"aws/claude-opus-5-5    1000k      5.00     25.00",
				"azure/gpt-5.6-sol       400k      1.25     10.00",
			},
		},
		{
			name:   "alias without target",
			models: []catalog.Model{alias},
			header: "MODEL                   CONTEXT      $ IN     $ OUT",
			rows:   []string{"aws/claude-opus-latest    1000k      5.00     25.00"},
		},
		{
			name:   "alias with target",
			models: []catalog.Model{alias, opus},
			opt:    Options{ShowAliasTarget: true},
			header: "MODEL                                      CONTEXT      $ IN     $ OUT",
			rows: []string{
				"aws/claude-opus-latest -> claude-opus-5-5    1000k      5.00     25.00",
				"aws/claude-opus-5-5                          1000k      5.00     25.00",
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
			header: "  MODEL                CONTEXT      $ IN     $ OUT",
			rows: []string{
				"  aws/claude-opus-5-5    1000k      5.00     25.00",
				"! azure/gpt-5.6-sol       400k      1.25     10.00",
			},
		},
		{
			name:   "no mark column without marks",
			models: []catalog.Model{opus},
			opt:    Options{Mark: func(catalog.Model) string { return "" }},
			header: "MODEL                CONTEXT      $ IN     $ OUT",
			rows:   []string{"aws/claude-opus-5-5    1000k      5.00     25.00"},
		},
		{
			name:   "benchmarks, multi-byte name",
			models: []catalog.Model{benchd, gpt},
			header: "MODEL              CONTEXT      $ IN     $ OUT  INTEL    TB4   CODE",
			rows: []string{
				"x/ä                     8k      0.00      0.00   57.6      -   80.0",
				"azure/gpt-5.6-sol     400k      1.25     10.00      -      -      -",
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
			for _, r := range rows {
				if utf8.RuneCountInString(r) != utf8.RuneCountInString(header) {
					t.Errorf("row %q not as wide as the header", r)
				}
			}
		})
	}
}

func TestFormatEmpty(t *testing.T) {
	header, rows := Format(nil, Options{})
	if header != "MODEL  CONTEXT      $ IN     $ OUT" || len(rows) != 0 {
		t.Errorf("Format(nil) = %q, %q", header, rows)
	}
}
