package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/schowave/llmenu/internal/atomicfile"
	"github.com/schowave/llmenu/internal/gateway"
)

// Spend caches each key's budget next to state.json, so a status line that
// Claude Code redraws after every message asks the gateway at most once a
// minute. Entries are keyed by a hash of gateway and key, never the key.
type Spend struct {
	Budget  gateway.Budget `json:"budget"`
	Fetched time.Time      `json:"fetched"`
}

// SpendID identifies a key at a gateway without revealing it.
func SpendID(baseURL, key string) string {
	h := sha256.Sum256([]byte(baseURL + "\x00" + key))
	return hex.EncodeToString(h[:12])
}

func spendPath() (string, error) {
	p, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "spend.json"), nil
}

func loadSpend() map[string]Spend {
	all := map[string]Spend{}
	p, err := spendPath()
	if err != nil {
		return all
	}
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	if all == nil {
		all = map[string]Spend{}
	}
	return all
}

// LoadSpend returns the cached budget for id, if any.
func LoadSpend(id string) (Spend, bool) {
	s, ok := loadSpend()[id]
	return s, ok
}

// SaveSpend records a budget; entries older than a day are dropped.
func SaveSpend(id string, b gateway.Budget, now time.Time) error {
	p, err := spendPath()
	if err != nil {
		return err
	}
	all := loadSpend()
	for k, s := range all {
		if now.Sub(s.Fetched) > 24*time.Hour {
			delete(all, k)
		}
	}
	all[id] = Spend{Budget: b, Fetched: now}
	b2, err := json.Marshal(all)
	if err != nil {
		return err
	}
	return atomicfile.Write(p, b2, 0o600)
}
