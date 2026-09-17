package mailer

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidInlines_dropsHeaderInjection(t *testing.T) {
	t.Parallel()
	kept := validInlines(context.Background(), []InlineImage{
		{Filename: "chart.png", MIMEType: "image/png", Data: []byte{1}},
		{Filename: "a\r\nContent-Type: text/html", MIMEType: "image/png", Data: []byte{2}},
		{Filename: "ok.png", MIMEType: "image/png\r\nX-Evil: 1", Data: []byte{3}},
		{Filename: "../escape.png", MIMEType: "image/png", Data: []byte{4}},
	})
	if len(kept) != 1 || kept[0].Filename != "chart.png" {
		t.Fatalf("kept = %+v, want only chart.png", kept)
	}
}

func TestBuildMIMEMessage_carriesInlineImage(t *testing.T) {
	t.Parallel()
	mime := string(buildMIMEMessage(
		"Name", "from@example.com", "to@example.com", "subject",
		"BOUND", "text", "<p>html</p>",
		[]InlineImage{{Filename: "chart.png", MIMEType: "image/png", Data: []byte("binary")}},
	))
	for _, want := range []string{
		`Content-Type: multipart/related; boundary="BOUND_rel"`,
		"Content-ID: <chart.png>",
		"Content-Transfer-Encoding: base64",
		base64.StdEncoding.EncodeToString([]byte("binary")),
	} {
		if !strings.Contains(mime, want) {
			t.Fatalf("mime missing %q:\n%s", want, mime)
		}
	}
}

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
