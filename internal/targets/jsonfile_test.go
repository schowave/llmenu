package targets

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
)

func setName(name string) func(any) any {
	return func(old any) any {
		m := asMap(old)
		m["name"] = name
		return m
	}
}

func TestEditJSONKeepsRest(t *testing.T) {
	orig := `{
  // comment stays
  "other": {"keep": true}, /* this one too */
  "providers": {
    "mine": {
      "name": "old",
      "extra": 1.50
    },
    "foreign": {"x": 1}
  }
}
`
	got, err := editJSON([]byte(orig), []string{"providers", "mine"}, setName("new"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  // comment stays
  "other": {"keep": true}, /* this one too */
  "providers": {
    "mine": {
      "extra": 1.50,
      "name": "new"
    },
    "foreign": {"x": 1}
  }
}
`
	if string(got) != want {
		t.Errorf("result:\n%s\nwant:\n%s", got, want)
	}
}

func TestEditJSONCreatesPath(t *testing.T) {
	orig := "{\n    \"$schema\": \"https://example.com/schema.json\" // Schema\n}\n"
	got, err := editJSON([]byte(orig), []string{"provider", "mine"}, setName("new"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{`"$schema"`, "// Schema", `"provider": {`, `"mine": {`, `"name": "new"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	want := "{\n    \"$schema\": \"https://example.com/schema.json\", // Schema\n" +
		"    \"provider\": {\n        \"mine\": {\n            \"name\": \"new\"\n        }\n    }\n}\n"
	if s != want {
		t.Errorf("result:\n%s\nwant:\n%s", s, want)
	}

	for _, orig := range []string{"{}", "{\n}\n", "{\n  \"a\": 1,\n}\n"} {
		got, err := editJSON([]byte(orig), []string{"p"}, setName("x"))
		if err != nil {
			t.Fatalf("%q: %v", orig, err)
		}
		if _, err := hujson.Standardize(got); err != nil {
			t.Errorf("%q yields invalid JSON:\n%s", orig, got)
		}
		if !strings.Contains(string(got), `"name": "x"`) {
			t.Errorf("%q: value missing:\n%s", orig, got)
		}
	}
}

func TestEditJSONInsertLayout(t *testing.T) {
	empty := func(any) any { return map[string]any{} }
	tests := []struct{ name, orig, want string }{
		{"blank object", "{  }", "{\n  \"p\": {}\n}"},
		{"comment in empty object stays", "{ // note\n}", "{ // note\n\n  \"p\": {}\n}"},
		{"space before trailing comma", "{\n  \"a\": 1 ,\n}\n", "{\n  \"a\": 1 ,\n  \"p\": {},\n}\n"},
		{"comment before trailing comma", "{\n  \"a\": 1 /* c */,\n}\n", "{\n  \"a\": 1 /* c */,\n  \"p\": {},\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := editJSON([]byte(tt.orig), []string{"p"}, empty)
			if err != nil || string(got) != tt.want {
				t.Errorf("editJSON(%q) = %q, %v; want %q", tt.orig, got, err, tt.want)
			}
		})
	}
}

func TestLineIndent(t *testing.T) {
	tests := []struct {
		b      string
		offset int
		want   string
	}{
		{"{\n    \"a\": 1}", 6, "    "},
		{"{\n\t\"a\": 1}", 3, "\t"},
		{"{\"a\": 1}", 1, ""},
		{"{\n  ", 4, "  "}, // whitespace up to the end of the input
	}
	for _, tt := range tests {
		if got := lineIndent([]byte(tt.b), tt.offset); got != tt.want {
			t.Errorf("lineIndent(%q, %d) = %q, want %q", tt.b, tt.offset, got, tt.want)
		}
	}
}

func TestEditJSONErrors(t *testing.T) {
	if _, err := editJSON([]byte(`[1]`), []string{"a"}, setName("x")); err == nil {
		t.Error("top-level array accepted")
	}
	if _, err := editJSON([]byte(`{"a": 1}`), []string{"a"}, setName("x")); err == nil {
		t.Error("number instead of object accepted")
	}
	if _, err := editJSON([]byte(`{"a": `), []string{"a"}, setName("x")); err == nil {
		t.Error("broken JSON accepted")
	}
	// Found by fuzzing: the inserted key never matched and the loop ran forever.
	if _, err := editJSON([]byte(`{}`), []string{"a", "\xff"}, setName("x")); err == nil {
		t.Error("invalid UTF-8 key accepted")
	}
	// Found by fuzzing: the first duplicate was edited, readers use the last.
	if _, err := editJSON([]byte(`{"p": {"a": {}}, "p": {}}`), []string{"p", "a"}, setName("x")); err == nil {
		t.Error("duplicate key accepted")
	}
}

func TestSyncJSONFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub", "conf.json")

	d, err := syncJSON(file, []byte("{}\n"), []string{"p"}, setName("a"), true)
	if err != nil || !d.Changed || d.Patch == "" {
		t.Fatalf("dry run: %+v %v", d, err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("dry run wrote")
	}

	if d, err = syncJSON(file, []byte("{}\n"), []string{"p"}, setName("a"), false); err != nil || !d.Changed {
		t.Fatalf("create: %+v %v", d, err)
	}
	if _, err := os.Stat(file + ".bak"); !os.IsNotExist(err) {
		t.Error("backup created for new file")
	}
	first, _ := os.ReadFile(file)

	if d, err = syncJSON(file, nil, []string{"p"}, setName("a"), false); err != nil || d.Changed {
		t.Fatalf("repeat changes: %+v %v", d, err)
	}

	if d, err = syncJSON(file, nil, []string{"p"}, setName("b"), false); err != nil || !d.Changed {
		t.Fatalf("change: %+v %v", d, err)
	}
	bak, _ := os.ReadFile(file + ".bak")
	if string(bak) != string(first) {
		t.Errorf("backup = %q, want %q", bak, first)
	}

	// The file's permissions are kept, also for an existing backup.
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(file, 0o640); err != nil { //nolint:gosec // G302: a mode other than the default
		t.Fatal(err)
	}
	// An existing backup with a different mode is aligned too.
	if err := os.Chmod(file+".bak", 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = syncJSON(file, nil, []string{"p"}, setName("c"), false); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{file, file + ".bak"} {
		if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o640 {
			t.Errorf("%s: mode %v, %v; want -rw-r-----", f, fi.Mode().Perm(), err)
		}
	}
}
