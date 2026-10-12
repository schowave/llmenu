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

func TestProjectKeys(t *testing.T) {
	keyring.MockInit()
	t.Setenv("LLMENU_API_KEY", "")

	if ProjectKey("") != Gateway || ProjectKey("acme") != "gateway:acme" {
		t.Errorf("ProjectKey: %q %q", ProjectKey(""), ProjectKey("acme"))
	}
	for _, ok := range []string{"acme", "a", "kunde-1", "x.y_z"} {
		if err := CheckProject(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "none", "Acme", "-x", ".x", "a/b", "a b", "+add"} {
		if err := CheckProject(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}

	if err := Set(ProjectKey("acme"), "sk-acme"); err != nil {
		t.Fatal(err)
	}
	if err := Set(Gateway, "sk-main"); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := Get(ProjectKey("acme")); v != "sk-acme" {
		t.Errorf("project key: %q", v)
	}
	if v, _, _ := Get(Gateway); v != "sk-main" {
		t.Errorf("main key changed: %q", v)
	}
	// LLMENU_API_KEY wins for project keys too, e.g. in CI.
	t.Setenv("LLMENU_API_KEY", "sk-env")
	if v, src, _ := Get(ProjectKey("acme")); v != "sk-env" || src != "environment" {
		t.Errorf("environment does not take precedence: %q %q", v, src)
	}
	if err := Set("gateway:Bad", "x"); err == nil {
		t.Error("invalid project name accepted")
	}
}

func TestFileStoreTightensExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix modes")
	}
	file := t.TempDir() + "/keys.json"
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil { //nolint:gosec // the point of the test
		t.Fatal(err)
	}
	t.Setenv("LLMENU_KEYRING_FILE", file)
	if err := Set(Gateway, "sk-file"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode: %v %v", fi, err)
	}
}
