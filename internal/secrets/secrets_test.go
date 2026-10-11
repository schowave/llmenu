package secrets

import (
	"os"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestGetSetDelete(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LLMENU_API_KEY", "")

	if v, src, err := Get(Gateway); v != "" || src != "" || err != nil {
		t.Fatalf("empty: %q %q %v", v, src, err)
	}
	if err := Set(Gateway, "sk-123"); err != nil {
		t.Fatal(err)
	}
	if v, src, _ := Get(Gateway); v != "sk-123" || src != "keyring" {
		t.Errorf("after Set: %q %q", v, src)
	}

	t.Setenv("LLMENU_API_KEY", "sk-env")
	if v, src, _ := Get(Gateway); v != "sk-env" || src != "environment" {
		t.Errorf("environment does not take precedence: %q %q", v, src)
	}
	t.Setenv("LLMENU_API_KEY", "")

	if err := Delete(Gateway); err != nil {
		t.Fatal(err)
	}
	if err := Delete(Gateway); err != nil {
		t.Errorf("second Delete: %v", err)
	}
	if v, _, _ := Get(Gateway); v != "" {
		t.Errorf("after Delete: %q", v)
	}

	if err := Set("mygateway", "x"); err == nil {
		t.Error("unknown name accepted")
	}
	if err := Set(Gateway, ""); err == nil {
		t.Error("empty key accepted")
	}
}

func TestFileStore(t *testing.T) {
	file := t.TempDir() + "/sub/keys.json"
	t.Setenv("LLMENU_KEYRING_FILE", file)
	t.Setenv("LLMENU_API_KEY", "")

	if v, src, err := Get(Gateway); v != "" || src != "" || err != nil {
		t.Fatalf("empty: %q %q %v", v, src, err)
	}
	if err := Set(Gateway, "sk-file"); err != nil {
		t.Fatal(err)
	}
	if v, src, _ := Get(Gateway); v != "sk-file" || src != "keyring file" {
		t.Errorf("after Set: %q %q", v, src)
	}
	if fi, err := os.Stat(file); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Errorf("file mode: %v %v", fi, err)
	}
	if err := Delete(Gateway); err != nil {
		t.Fatal(err)
	}
	if err := Delete(Gateway); err != nil {
		t.Errorf("second Delete: %v", err)
	}
	if v, _, _ := Get(Gateway); v != "" {
		t.Errorf("after Delete: %q", v)
	}

	if err := os.WriteFile(file, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Get(Gateway); err == nil {
		t.Error("corrupt file not reported")
	}
	if err := Set(Gateway, "x"); err == nil {
		t.Error("corrupt file overwritten")
	}
}

func TestServiceIsolation(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LLMENU_API_KEY", "")

	// A key stored under one service must not be visible under another, so
	// tests with their own LLMENU_KEYRING_SERVICE never touch the real entry.
	t.Setenv("LLMENU_KEYRING_SERVICE", "llmenu-test-a")
	if err := Set(Gateway, "sk-a"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LLMENU_KEYRING_SERVICE", "llmenu-test-b")
	if v, _, _ := Get(Gateway); v != "" {
		t.Errorf("key leaked across services: %q", v)
	}
	t.Setenv("LLMENU_KEYRING_SERVICE", "")
	if v, _, _ := Get(Gateway); v != "" {
		t.Errorf("key leaked into the default service: %q", v)
	}
}
