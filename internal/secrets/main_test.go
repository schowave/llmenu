package secrets

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain swaps the OS keyring for an in-memory one for the whole package,
// so no test — and no mutant that loses an override — can reach the real
// keychain.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}
