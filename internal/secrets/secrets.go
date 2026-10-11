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
	"slices"

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

func check(name string) error {
	if _, ok := Names[name]; !ok {
		var known []string
		for n := range Names {
			known = append(known, n)
		}
		slices.Sort(known)
		return fmt.Errorf("unknown key %q, known: %v", name, known)
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
	if v := os.Getenv(Names[name]); v != "" {
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
	return os.WriteFile(string(f), b, 0o600)
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
