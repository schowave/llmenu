// Package fetch retrieves JSON data via HTTP GET with readable errors: on an
// error status, the status and the start of the response are included.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Get requests u with the headers; a nil hc means a default client with a 30 s timeout.
func Get(ctx context.Context, hc *http.Client, u string, header map[string]string) ([]byte, error) {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("request to %s aborted: %w", u, err)
	}
	if res.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d from %s: %s", res.StatusCode, u, snippet(body, 300))
	}
	return body, nil
}

func snippet(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
