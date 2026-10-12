// Package secrets manages API keys in the operating system's keyring
// (macOS Keychain, Windows Credential Manager, Secret Service on Linux).
// Environment variables take precedence, e.g. for CI.
package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/schowave/llmenu/internal/guard"
)

const (
	Gateway            = "gateway"
	ArtificialAnalysis = "artificialanalysis"
)

// Names are the known entries with the environment variable that overrides each.
var Names = map[string]string{
	Gateway:            "LLMENU_API_KEY",
	ArtificialAnalysis: "LLMENU_AA_API_KEY",
}

// service can be changed via LLMENU_KEYRING_SERVICE so tests do not touch the
// real entry.
func service() string {
	if s := os.Getenv("LLMENU_KEYRING_SERVICE"); s != "" {
		return s
	}
	return "llmenu"
}

// projectPrefix marks a project's gateway key: gateway:<project>.
const projectPrefix = Gateway + ":"

// NoProject is the reserved name for "no project, the main key", e.g. in
// LLMENU_PROJECT=none.
const NoProject = "none"

var reProject = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ProjectKey is the entry for a project's gateway key; "" is the main key.
func ProjectKey(project string) string {
	if project == "" {
		return Gateway
	}
	return projectPrefix + project
}

// CheckProject validates a project name: lower case letters, digits, ".",
// "_" and "-", not starting with a punctuation mark, and not "none".
func CheckProject(name string) error {
	if name == NoProject {
		return fmt.Errorf("%q is reserved for the main key", NoProject)
	}
	if !reProject.MatchString(name) {
		return fmt.Errorf("invalid project name %q: use a-z, 0-9, '.', '_' and '-'", name)
	}
	return nil
}

// envVar is the variable that overrides an entry; project keys share the
// gateway's, so LLMENU_API_KEY always wins, e.g. in CI.
func envVar(name string) string {
	if strings.HasPrefix(name, projectPrefix) {
		return Names[Gateway]
	}
	return Names[name]
}

func check(name string) error {
	if p, ok := strings.CutPrefix(name, projectPrefix); ok {
		return CheckProject(p)
	}
	if _, ok := Names[name]; !ok {
		var known []string
		for n := range Names {
			known = append(known, n)
		}
		slices.Sort(known)
		return fmt.Errorf("unknown key %q, known: %v and gateway:<project>", name, known)
	}
	return nil
}

// Get returns the key and where it comes from ("environment", "keyring" or
// "keyring file").
// If it is set nowhere, value is empty and err is nil.
func Get(name string) (value, source string, err error) {
	if err := check(name); err != nil {
		return "", "", err
	}
	if v := os.Getenv(envVar(name)); v != "" {
		return v, "environment", nil
	}
	st := backend()
	v, err := st.Get(service(), name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("keyring: %w", err)
	}
	return v, st.label(), nil
}

// Set stores the key in the keyring.
func Set(name, value string) error {
	if err := check(name); err != nil {
		return err
	}
	if value == "" {
		return errors.New("empty key")
	}
	if err := backend().Set(service(), name, value); err != nil {
		return fmt.Errorf("keyring: %w", err)
	}
	return nil
}

// Delete removes the key; a missing key is not an error.
func Delete(name string) error {
	if err := check(name); err != nil {
		return err
	}
	err := backend().Delete(service(), name)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keyring: %w", err)
	}
	return nil
}

// store is where keys live: the OS keyring, or for sandboxes and tests a
// plain file (LLMENU_KEYRING_FILE).
type store interface {
	Get(service, name string) (string, error)
	Set(service, name, value string) error
	Delete(service, name string) error
	label() string
}

func backend() store {
	if f := os.Getenv("LLMENU_KEYRING_FILE"); f != "" {
		return fileStore(f)
	}
	return osStore{}
}

type osStore struct{}

func (osStore) Get(service, name string) (string, error) { return keyring.Get(service, name) }
func (osStore) Set(service, name, value string) error    { return keyring.Set(service, name, value) }
func (osStore) Delete(service, name string) error        { return keyring.Delete(service, name) }
func (osStore) label() string                            { return "keyring" }

// fileStore keeps keys in plain text in a JSON file (mode 0600). Only meant
// for the development sandbox and tests, never for real keys.
type fileStore string

func (f fileStore) label() string { return "keyring file" }

func (f fileStore) load() (map[string]string, error) {
	m := map[string]string{}
	if _, err := guard.Path(string(f)); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(string(f))
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", string(f), err)
	}
	return m, nil
}

func (f fileStore) save(m map[string]string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(string(f)), 0o700); err != nil {
		return err
	}
	// Via a new file: WriteFile would keep the mode of an existing, possibly
	// world-readable one.
	tmp, err := os.CreateTemp(filepath.Dir(string(f)), ".keyring-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), string(f)) // CreateTemp files are 0600
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

func (f fileStore) Get(service, name string) (string, error) {
	m, err := f.load()
	if err != nil {
		return "", err
	}
	v, ok := m[service+"/"+name]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (f fileStore) Set(service, name, value string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	m[service+"/"+name] = value
	return f.save(m)
}

func (f fileStore) Delete(service, name string) error {
	m, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := m[service+"/"+name]; !ok {
		return keyring.ErrNotFound
	}
	delete(m, service+"/"+name)
	return f.save(m)
}
