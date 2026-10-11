package secrets

import (
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
