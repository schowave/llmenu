package targets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/tailscale/hujson"

	"github.com/schowave/llmenu/internal/atomicfile"
	"github.com/schowave/llmenu/internal/guard"
)

// syncJSON replaces the value under path (e.g. ["providers", "mygateway"]) in
// another tool's JSON or JSONC file and leaves everything else byte-for-byte
// intact, comments included (ADR-0002, sync rules). If the file is missing, it
// is created from initial. It writes only on changes, atomically and with a
// backup of the old version as *.bak.
func syncJSON(file string, initial []byte, path []string, update func(old any) any, dryRun bool) (Diff, error) {
	d := Diff{Path: file}
	if _, err := guard.Path(file); err != nil {
		return d, err
	}
	orig, err := os.ReadFile(file)
	exists := err == nil
	if os.IsNotExist(err) {
		orig = initial
	} else if err != nil {
		return d, err
	}

	updated, err := editJSON(orig, path, update)
	if err != nil {
		return d, fmt.Errorf("%s: %w", file, err)
	}
	if exists && bytes.Equal(orig, updated) {
		return d, nil
	}
	d.Changed = true
	var before []byte
	if exists {
		before = orig
	}
	d.Patch, _ = difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(string(before)), B: difflib.SplitLines(string(updated)),
		FromFile: file, ToFile: file, Context: 3,
	})
	if dryRun {
		return d, nil
	}

	mode := os.FileMode(0o600)
	if exists {
		if fi, err := os.Stat(file); err == nil {
			mode = fi.Mode().Perm()
		}
		// The backup must not be more open than the config itself.
		if err := atomicfile.Write(file+".bak", orig, mode); err != nil {
			return d, fmt.Errorf("backup: %w", err)
		}
	}
	return d, atomicfile.Write(file, updated, mode)
}

// editJSON returns orig with a new value under path. Only text is replaced or
// inserted; formatting and comments in the rest of the file stay.
func editJSON(orig []byte, path []string, update func(old any) any) ([]byte, error) {
	for _, k := range path {
		// JSON cannot hold such a key, the inserted member would never match.
		if !utf8.ValidString(k) {
			return nil, fmt.Errorf("key %q is not valid UTF-8", k)
		}
	}
	v, err := hujson.Parse(orig)
	if err != nil {
		return nil, err
	}
	if _, ok := v.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("top level is not a JSON object")
	}

	ptr := pointer(path)
	for v.Find(ptr) == nil {
		k := 0
		for k < len(path) && v.Find(pointer(path[:k+1])) != nil {
			k++
		}
		parent := v.Find(pointer(path[:k])) // the empty pointer is v itself
		obj, ok := parent.Value.(*hujson.Object)
		if !ok {
			return nil, fmt.Errorf("%s is not a JSON object", pointer(path[:k]))
		}
		orig = insertMember(orig, *parent, obj, path[k])
		if v, err = hujson.Parse(orig); err != nil {
			return nil, err
		}
		if v.Find(pointer(path[:k+1])) == nil {
			return nil, fmt.Errorf("cannot insert %s", pointer(path[:k+1]))
		}
	}

	if err := checkUnique(&v, path); err != nil {
		return nil, err
	}
	target := v.Find(ptr)
	if _, ok := target.Value.(*hujson.Object); !ok {
		return nil, fmt.Errorf("%s is not a JSON object", ptr)
	}
	old, err := decode(*target)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(lineIndent(orig, target.StartOffset), indentUnit(orig))
	if err := enc.Encode(update(old)); err != nil {
		return nil, err
	}
	text := bytes.TrimRight(buf.Bytes(), "\n")

	out := make([]byte, 0, len(orig)+len(text))
	out = append(out, orig[:target.StartOffset]...)
	out = append(out, text...)
	return append(out, orig[target.EndOffset:]...), nil
}

// checkUnique fails if a key along path occurs more than once in its object:
// Find edits the first occurrence, but JSON readers use the last one.
func checkUnique(v *hujson.Value, path []string) error {
	for i, key := range path {
		obj, ok := v.Find(pointer(path[:i])).Value.(*hujson.Object)
		if !ok {
			return nil // reported by the caller
		}
		n := 0
		for _, m := range obj.Members {
			if name, ok := m.Name.Value.(hujson.Literal); ok && name.String() == key {
				n++
			}
		}
		if n > 1 {
			return fmt.Errorf("duplicate key %s", pointer(path[:i+1]))
		}
	}
	return nil
}

// insertMember inserts a member key with an empty object as value into obj,
// after the last member and with its indentation.
func insertMember(orig []byte, parent hujson.Value, obj *hujson.Object, key string) []byte {
	name, _ := json.Marshal(key)
	member := string(name) + ": {}"
	open, closing := parent.StartOffset, parent.EndOffset-1
	outer := lineIndent(orig, parent.StartOffset)

	splice := func(at int, text string) []byte {
		out := append([]byte{}, orig[:at]...)
		out = append(out, text...)
		return append(out, orig[at:]...)
	}
	if len(obj.Members) == 0 {
		inner := orig[open+1 : closing]
		text := "\n" + outer + indentUnit(orig) + member + "\n" + outer
		if len(bytes.TrimSpace(inner)) == 0 {
			out := append([]byte{}, orig[:open+1]...)
			out = append(out, text...)
			return append(out, orig[closing:]...)
		}
		return splice(closing, text)
	}

	indent := lineIndent(orig, obj.Members[0].Name.StartOffset)
	last := obj.Members[len(obj.Members)-1].Value
	if pos := last.EndOffset + len(last.AfterExtra); pos < len(orig) && orig[pos] == ',' {
		return splice(pos+1, "\n"+indent+member+",") // JSONC with trailing comma
	}
	// Comma right after the value, the new member at the end of the line, so a
	// trailing comment stays with its value.
	lineEnd := last.EndOffset
	if i := bytes.IndexByte(orig[last.EndOffset:closing], '\n'); i >= 0 {
		lineEnd += i
	}
	out := append([]byte{}, orig[:last.EndOffset]...)
	out = append(out, ',')
	out = append(out, orig[last.EndOffset:lineEnd]...)
	out = append(out, "\n"+indent+member...)
	return append(out, orig[lineEnd:]...)
}

// decode reads a hujson value as plain JSON; numbers stay exact.
func decode(v hujson.Value) (any, error) {
	c := v.Clone()
	c.Standardize()
	dec := json.NewDecoder(bytes.NewReader(c.Pack()))
	dec.UseNumber()
	var out any
	return out, dec.Decode(&out)
}

func pointer(path []string) string {
	r := strings.NewReplacer("~", "~0", "/", "~1")
	var b strings.Builder
	for _, p := range path {
		b.WriteString("/" + r.Replace(p))
	}
	return b.String()
}

var reIndent = regexp.MustCompile(`(?m)^([ \t]+)\S`)

// indentUnit is the indentation of the first indented line, otherwise two spaces.
func indentUnit(b []byte) string {
	if m := reIndent.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return "  "
}

// lineIndent is the leading indentation of the line containing offset.
func lineIndent(b []byte, offset int) string {
	start := bytes.LastIndexByte(b[:offset], '\n') + 1
	end := start
	for end < len(b) && (b[end] == ' ' || b[end] == '\t') {
		end++
	}
	return string(b[start:end])
}

// asMap returns a JSON object as a map; anything else as an empty map.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
