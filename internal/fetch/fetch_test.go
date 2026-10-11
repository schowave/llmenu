package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGet(t *testing.T) {
	long := strings.Repeat("ä", 400)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if r.Header.Get("Authorization") != "Bearer k" || r.Header.Get("X-Extra") != "1" {
				http.Error(w, "headers missing", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/short":
			http.Error(w, "  invalid key  ", http.StatusUnauthorized)
		case "/long":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(long))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	header := map[string]string{"Authorization": "Bearer k", "X-Extra": "1"}

	body, err := Get(context.Background(), nil, srv.URL+"/ok", header)
	if err != nil || string(body) != `{"ok":true}` {
		t.Errorf("success: %q, %v", body, err)
	}
	if _, err := Get(context.Background(), srv.Client(), srv.URL+"/ok", nil); err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("without headers: %v", err)
	}

	_, err = Get(context.Background(), nil, srv.URL+"/short", nil)
	if want := "HTTP 401 from " + srv.URL + "/short: invalid key"; err == nil || err.Error() != want {
		t.Errorf("error status: %v, want %q", err, want)
	}

	_, err = Get(context.Background(), nil, srv.URL+"/long", nil)
	if err == nil {
		t.Fatal("HTTP 500 accepted")
	}
	_, snip, _ := strings.Cut(err.Error(), "/long: ")
	if !strings.HasSuffix(snip, "…") || utf8.RuneCountInString(snip) != 301 || !strings.HasPrefix(long, strings.TrimSuffix(snip, "…")) {
		t.Errorf("snippet not cut to 300 runes: %q", snip)
	}
}

func TestGetErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nobody listens any more

	if _, err := Get(context.Background(), nil, url, nil); err == nil || !strings.HasPrefix(err.Error(), "request failed:") {
		t.Errorf("connection error: %v", err)
	}
	if _, err := Get(context.Background(), nil, "://bad", nil); err == nil {
		t.Error("invalid URL accepted")
	}

	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("only a part"))
	}))
	defer cut.Close()
	if _, err := Get(context.Background(), nil, cut.URL, nil); err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Errorf("cut-off body: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Get(ctx, nil, url, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v", err)
	}
}

func TestSnippet(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"", 3, ""},
		{"  abc \n", 3, "abc"},
		{"abcd", 3, "abc…"},
		{"äöüß", 2, "äö…"},
	}
	for _, tt := range tests {
		if got := snippet([]byte(tt.in), tt.n); got != tt.want {
			t.Errorf("snippet(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
