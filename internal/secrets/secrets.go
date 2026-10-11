// Package secrets manages API keys in the operating system's keyring
// (macOS Keychain, Windows Credential Manager, Secret Service on Linux).
// Environment variables take precedence, e.g. for CI.
package secrets

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/zalando/go-keyring"
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

// Get returns the key and where it comes from ("environment" or "keyring").
// If it is set nowhere, value is empty and err is nil.
func Get(name string) (value, source string, err error) {
	if err := check(name); err != nil {
		return "", "", err
	}
	if v := os.Getenv(Names[name]); v != "" {
		return v, "environment", nil
	}
	v, err := keyring.Get(service(), name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("keyring: %w", err)
	}
	return v, "keyring", nil
}

// Set stores the key in the keyring.
func Set(name, value string) error {
	if err := check(name); err != nil {
		return err
	}
	if value == "" {
		return errors.New("empty key")
	}
	if err := keyring.Set(service(), name, value); err != nil {
		return fmt.Errorf("keyring: %w", err)
	}
	return nil
}

// Delete removes the key; a missing key is not an error.
func Delete(name string) error {
	if err := check(name); err != nil {
		return err
	}
	err := keyring.Delete(service(), name)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keyring: %w", err)
	}
	return nil
}
