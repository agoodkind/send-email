package mailer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAPIKeyFromEnvFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "w.env")
	if err := os.WriteFile(p, []byte("# c\nSMTP2GO_API_KEY=abc123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadAPIKeyFromEnvFiles([]string{filepath.Join(dir, "missing"), p})
	if got != "abc123" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadAPIKeyFromEnvFiles_quoted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "w.env")
	if err := os.WriteFile(p, []byte(`SMTP2GO_API_KEY="xyz"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := LoadAPIKeyFromEnvFiles([]string{p})
	if got != "xyz" {
		t.Fatalf("got %q", got)
	}
}

func TestMailerMethod_explicitHTTP(t *testing.T) {
	t.Parallel()
	m := New(Config{Transport: MethodHTTP, SMTP2GOAPIKey: "k"})
	if m.Method() != "http" {
		t.Fatalf("got %q", m.Method())
	}
}

func TestMailerMethod_explicitSendmail(t *testing.T) {
	t.Parallel()
	m := New(Config{Transport: MethodSendmail})
	if m.Method() != "sendmail" {
		t.Fatalf("got %q", m.Method())
	}
}

func TestMailerMethod_autoFromEnv(t *testing.T) {
	t.Setenv("SMTP2GO_API_KEY", "fromenv")
	m := New(Config{Transport: MethodAuto})
	if m.Method() != "http" {
		t.Fatalf("got %q", m.Method())
	}
}

func TestMailerMethod_autoNoKey(t *testing.T) {
	t.Setenv("SMTP2GO_API_KEY", "")
	m := New(Config{Transport: MethodAuto, SMTP2GOAPIKey: ""})
	if m.Method() != "sendmail" {
		t.Fatalf("got %q", m.Method())
	}
}

func TestParseBoolEnv(t *testing.T) {
	t.Parallel()
	if !ParseBoolEnv("true") || !ParseBoolEnv("YES") || !ParseBoolEnv("1") {
		t.Fatal("expected true")
	}
	if ParseBoolEnv("no") || ParseBoolEnv("") {
		t.Fatal("expected false")
	}
}

func TestAtoiDefault(t *testing.T) {
	t.Parallel()
	if AtoiDefault("", 7) != 7 {
		t.Fatal()
	}
	if AtoiDefault("12", 0) != 12 {
		t.Fatal()
	}
	if AtoiDefault("x", 3) != 3 {
		t.Fatal()
	}
}

func TestSend_HTTPRequiresKey(t *testing.T) {
	t.Setenv("SMTP2GO_API_KEY", "")
	m := New(Config{Transport: MethodHTTP, SMTP2GOAPIKey: ""})
	err := m.Send(context.Background(), Message{To: "a@b.c", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "SMTP2GO") {
		t.Fatalf("expected key error, got %v", err)
	}
}
