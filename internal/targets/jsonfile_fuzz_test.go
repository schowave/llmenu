package targets

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tailscale/hujson"
)

// FuzzEditJSON checks editJSON's promises for any input: the result is valid
// JSONC, holds update's value under path and keeps the other members.
func FuzzEditJSON(f *testing.F) {
	for _, seed := range []string{
		"{}", "{\n}\n", "{\n  \"a\": 1,\n}\n", `[1]`, `{"a": 1}`, `{"a": `,
		"{\n    \"$schema\": \"https://example.com/schema.json\" // Schema\n}\n",
		"{\n  // comment stays\n  \"other\": {\"keep\": true}, /* this one too */\n  \"providers\": {\n" +
			"    \"mine\": {\n      \"name\": \"old\",\n      \"extra\": 1.50\n    },\n    \"foreign\": {\"x\": 1}\n  }\n}\n",
		opencodeInitial,
		`{"providers": {"mine": {}}, "providers": {"other": {}}}`,
	} {
		f.Add([]byte(seed), "providers", "mine", true, "new")
		f.Add([]byte(seed), "p", "", false, "x")
	}

	f.Fuzz(func(t *testing.T, orig []byte, key1, key2 string, deep bool, name string) {
		path := []string{key1}
		if deep {
			path = append(path, key2)
		}
		var want any
		update := func(old any) any {
			m := asMap(old)
			m["name"] = name
			want = m
			return m
		}

		var got []byte
		var err error
		done := make(chan struct{})
		go func() { got, err = editJSON(bytes.Clone(orig), path, update); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("HANG %q %q", orig, path)
		}
		before, parseErr := standardJSON(orig)
		if parseErr != nil {
			if err == nil {
				t.Fatalf("invalid input accepted: %q", orig)
			}
			return
		}
		if err != nil {
			if want != nil {
				t.Fatalf("update called, but error: %v", err)
			}
			if !blockedPath(before, path) && !slices.ContainsFunc(path, func(k string) bool { return !utf8.ValidString(k) }) &&
				!strings.Contains(err.Error(), "duplicate key") {
				t.Fatalf("editJSON(%q, %q): %v", orig, path, err)
			}
			return
		}

		after, err := standardJSON(got)
		if err != nil {
			t.Fatalf("result is invalid: %v\ninput:\n%s\nresult:\n%s", err, orig, got)
		}
		if v, ok := lookup(after, path); !ok || !reflect.DeepEqual(v, roundTrip(t, want)) {
			t.Fatalf("value under %q = %#v, want %#v\ninput:\n%s\nresult:\n%s", path, v, want, orig, got)
		}
		// Siblings along the path stay as they were.
		for i := range path {
			b, _ := lookup(before, path[:i])
			a, _ := lookup(after, path[:i])
			bm, am := asMap(b), asMap(a)
			for k, v := range bm {
				if k != path[i] && !reflect.DeepEqual(am[k], v) {
					t.Fatalf("member %q of %q changed: %#v -> %#v\ninput:\n%s\nresult:\n%s", k, path[:i], v, am[k], orig, got)
				}
			}
			if _, existed := bm[path[i]]; len(am) != len(bm)+boolInt(!existed) {
				t.Fatalf("members of %q: %d -> %d\ninput:\n%s\nresult:\n%s", path[:i], len(bm), len(am), orig, got)
			}
		}
	})
}

// standardJSON parses JSONC like a reading tool would: last duplicate wins.
func standardJSON(b []byte) (any, error) {
	std, err := hujson.Standardize(bytes.Clone(b))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(std))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func roundTrip(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	out, err := standardJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func lookup(v any, path []string) (any, bool) {
	for _, k := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = m[k]; !ok {
			return nil, false
		}
	}
	return v, true
}

// blockedPath reports whether a step of path is present, but not an object.
func blockedPath(v any, path []string) bool {
	for i := 0; i <= len(path); i++ {
		w, ok := lookup(v, path[:i])
		if !ok {
			return false
		}
		if _, isObj := w.(map[string]any); !isObj {
			return true
		}
	}
	return false
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
