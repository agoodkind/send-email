package mailer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		[]Attachment{{Filename: "trace.txt", MIMEType: "text/plain", Data: []byte("diagnostics")}},
	))
	for _, want := range []string{
		`Content-Type: multipart/mixed; boundary="BOUND_mixed"`,
		`Content-Type: multipart/related; boundary="BOUND_rel"; type="multipart/alternative"`,
		"Content-ID: <chart.png>",
		`Content-Disposition: attachment; filename="trace.txt"`,
		"Content-Transfer-Encoding: base64",
		base64.StdEncoding.EncodeToString([]byte("binary")),
		base64.StdEncoding.EncodeToString([]byte("diagnostics")),
	} {
		if !strings.Contains(mime, want) {
			t.Fatalf("mime missing %q:\n%s", want, mime)
		}
	}
}

func TestSend_HTTPIncludesAttachment(t *testing.T) {
	type requestPayload struct {
		Inlines     []smtp2goFile `json:"inlines"`
		Attachments []smtp2goFile `json:"attachments"`
	}
	requests := make(chan struct {
		Path        string
		Method      string
		ContentType string
		Payload     requestPayload
		Err         error
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload requestPayload
		err := json.NewDecoder(request.Body).Decode(&payload)
		requests <- struct {
			Path        string
			Method      string
			ContentType string
			Payload     requestPayload
			Err         error
		}{request.URL.Path, request.Method, request.Header.Get("Content-Type"), payload, err}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"succeeded":1}}`))
	}))
	defer server.Close()

	message := Message{
		To: "recipient@example.com", Subject: "diagnostics", Body: "See attachment",
		Inlines:     []InlineImage{{Filename: "chart.png", MIMEType: "image/png", Data: []byte{1, 2}}},
		Attachments: []Attachment{{Filename: "trace.txt", MIMEType: "text/plain", Data: []byte("packet loss\n")}},
	}
	mailer := New(Config{Transport: MethodHTTP, SMTP2GOAPIKey: "test", SMTP2GOEndpoint: server.URL + "/v3/email/send"})
	if err := mailer.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	received := <-requests
	if received.Err != nil {
		t.Fatal(received.Err)
	}
	if received.Path != "/v3/email/send" || received.Method != http.MethodPost || received.ContentType != "application/json" {
		t.Fatalf("The request was %s %s %s", received.Method, received.Path, received.ContentType)
	}
	if len(received.Payload.Attachments) != 1 {
		t.Fatalf("The request included attachments %+v", received.Payload.Attachments)
	}
	attachment := received.Payload.Attachments[0]
	if attachment.Filename != "trace.txt" || attachment.MIMEType != "text/plain" || attachment.FileBlob != base64.StdEncoding.EncodeToString([]byte("packet loss\n")) {
		t.Fatalf("The attachment was %+v", attachment)
	}
	if len(received.Payload.Inlines) != 1 || received.Payload.Inlines[0].Filename != "chart.png" {
		t.Fatalf("The request included inline images %+v", received.Payload.Inlines)
	}
}

func TestSendRejectsInvalidAttachmentMetadata(t *testing.T) {
	mailer := New(Config{Transport: MethodHTTP, SMTP2GOAPIKey: "test"})
	for _, test := range []struct {
		name       string
		attachment Attachment
		want       string
	}{
		{name: "filename", attachment: Attachment{Filename: "trace\n.txt", MIMEType: "text/plain", Data: nil}, want: "attachment filename"},
		{name: "media type", attachment: Attachment{Filename: "trace.txt", MIMEType: "text/plain; bad", Data: nil}, want: "attachment mimetype"},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := Message{To: "recipient@example.com", Subject: "diagnostics", Body: "See attachment", Attachments: []Attachment{test.attachment}}
			err := mailer.Send(context.Background(), message)
			if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "inline") {
				t.Fatalf("The send error was %v; it should contain %q without inline wording", err, test.want)
			}
		})
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
