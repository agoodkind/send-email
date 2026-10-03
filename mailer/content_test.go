package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func orderedTestMessage() Message {
	return Message{To: "recipient@example.com", Subject: "ordered", Content: []ContentBlock{
		{Kind: ContentText, Text: "first <unsafe>"},
		{Kind: ContentHTML, HTML: "<p>second markup</p>", Text: "second alternative"},
		{Kind: ContentTable, Table: &Table{Caption: "third table", Headers: []string{"<header>"}, Rows: [][]string{{"<cell>"}}}},
		{Kind: ContentHTML, HTML: "<p>fourth markup</p>", Text: "fourth alternative"},
		{Kind: ContentTable, Table: &Table{Caption: "fifth table", Headers: []string{"last header"}, Rows: [][]string{{"last cell"}}}},
	}}
}

func assertContentAlternatives(t *testing.T, plain, html, preview string) {
	t.Helper()
	preheaderStart := strings.Index(html, `<div style="display:none;`)
	if preheaderStart < 0 {
		t.Fatal("missing email preview")
	}
	preheaderEnd := strings.Index(html[preheaderStart:], "</div>") + preheaderStart
	if preheaderEnd < preheaderStart || !strings.Contains(html[preheaderStart:preheaderEnd], stdhtml.EscapeString(normalizePreheader(preview))) {
		t.Fatal("email preview omitted first content")
	}
	html = html[:preheaderStart] + html[preheaderEnd+len("</div>"):]
	for _, body := range []struct {
		text   string
		tokens []string
	}{
		{plain, []string{"first <unsafe>", "second alternative", "third table", "fourth alternative", "fifth table", "Caller:"}},
		{html, []string{"first &lt;unsafe&gt;", "second markup", "third table", "fourth markup", "fifth table", `class="meta"`}},
	} {
		last := -1
		for _, token := range body.tokens {
			position := strings.Index(body.text, token)
			if position <= last || strings.Count(body.text, token) != 1 {
				t.Fatalf("invalid order or duplicate %q in %s", token, body.text)
			}
			last = position
		}
	}
	if !strings.Contains(html, "&lt;header&gt;") || !strings.Contains(html, "&lt;cell&gt;") {
		t.Fatal("table content was not escaped")
	}
	if strings.Contains(plain, "second markup") || strings.Contains(html, "second alternative") {
		t.Fatal("alternatives duplicated content")
	}
}

func TestSendHTTPOrderedContent(t *testing.T) {
	requests := make(chan struct{ plain, html string }, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			Text string `json:"text_body"`
			HTML string `json:"html_body"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		requests <- struct{ plain, html string }{payload.Text, payload.HTML}
		_, _ = writer.Write([]byte(`{"data":{"succeeded":1}}`))
	}))
	defer server.Close()
	mailer := New(Config{Transport: MethodHTTP, SMTP2GOAPIKey: "test", SMTP2GOEndpoint: server.URL})
	for _, preview := range []string{"", "custom <preview>\nnext"} {
		message := orderedTestMessage()
		message.Preheader = preview
		if err := mailer.Send(context.Background(), message); err != nil {
			t.Fatal(err)
		}
		received := <-requests
		if preview == "" {
			preview = "first <unsafe>"
		}
		assertContentAlternatives(t, received.plain, received.html, preview)
	}
}

func TestSendSMTPOrderedContent(t *testing.T) {
	for _, preview := range []string{"", "custom <preview>\nnext"} {
		message := orderedTestMessage()
		message.Preheader = preview
		plain, html := sendContentSMTPMessage(t, message)
		if preview == "" {
			preview = "first <unsafe>"
		}
		assertContentAlternatives(t, plain, html, preview)
	}
}

func sendContentSMTPMessage(t *testing.T, outbound Message) (string, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan []byte, 1)
	errors := make(chan error, 1)
	go func() { errors <- receiveContentSMTP(listener, result) }()
	port := listener.Addr().(*net.TCPAddr).Port
	config := filepath.Join(t.TempDir(), "msmtprc")
	if err := os.WriteFile(config, fmt.Appendf(nil, "account local\nhost 127.0.0.1\nport %d\nfrom sender@example.com\nuser test\npassword test\ntls off\ntls_starttls off\naccount default : local\n", port), 0o600); err != nil {
		t.Fatal(err)
	}
	mailer := New(Config{Transport: MethodSendmail, MsmtprcPath: config})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mailer.Send(ctx, outbound); err != nil {
		t.Fatal(err)
	}
	if err := <-errors; err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(<-result))
	if err != nil {
		t.Fatal(err)
	}
	_, parameters, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	parts := multipart.NewReader(message.Body, parameters["boundary"])
	plain, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	plainBytes, err := io.ReadAll(plain)
	if err != nil {
		t.Fatal(err)
	}
	html, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	htmlBytes, err := io.ReadAll(html)
	if err != nil {
		t.Fatal(err)
	}
	return string(plainBytes), string(htmlBytes)
}

func TestSendSMTPPreheaderPreservesLegacyBody(t *testing.T) {
	for _, preview := range []string{"", "custom <preview>\nnext"} {
		body := "legacy <body>\nsecond"
		plain, html := sendContentSMTPMessage(t, Message{To: "recipient@example.com", Body: body, Preheader: preview})
		if preview == "" {
			preview = body
		}
		start := strings.Index(html, `<div style="display:none;`)
		end := start + strings.Index(html[start:], "</div>")
		if !strings.Contains(html[start:end], stdhtml.EscapeString(normalizePreheader(preview))) {
			t.Fatal("legacy preview was not preserved or overridden")
		}
		if !strings.Contains(plain, body) || strings.Count(plain, "Caller:") != 1 || strings.Count(html, `class="meta"`) != 1 {
			t.Fatal("legacy body or metadata changed")
		}
	}
}

func receiveContentSMTP(listener net.Listener, result chan<- []byte) error {
	connection, err := listener.Accept()
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	protocol := textproto.NewConn(connection)
	if err := protocol.PrintfLine("220 localhost SMTP"); err != nil {
		return err
	}
	for {
		line, err := protocol.ReadLine()
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(line, "EHLO"):
			err = protocol.PrintfLine("250-localhost\r\n250 AUTH PLAIN")
		case strings.HasPrefix(line, "AUTH"):
			err = protocol.PrintfLine("235 authenticated")
		case line == "DATA":
			if err = protocol.PrintfLine("354 send data"); err != nil {
				return err
			}
			data, readErr := io.ReadAll(protocol.DotReader())
			if readErr != nil {
				return readErr
			}
			result <- data
			err = protocol.PrintfLine("250 accepted")
		case line == "QUIT":
			return protocol.PrintfLine("221 closing")
		default:
			err = protocol.PrintfLine("250 accepted")
		}
		if err != nil {
			return err
		}
	}
}

func TestSendRejectsInvalidContent(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- struct{}{}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	for _, message := range []Message{
		{Body: "legacy", Content: []ContentBlock{{Kind: ContentText, Text: "new"}}},
		{HTML: "<p>legacy</p>", Content: []ContentBlock{{Kind: ContentText, Text: "new"}}},
		{Tables: []Table{{Caption: "legacy"}}, Content: []ContentBlock{{Kind: ContentText, Text: "new"}}},
		{Content: []ContentBlock{{Kind: ContentHTML, HTML: "<p>missing alternative</p>"}}},
		{Content: []ContentBlock{{Kind: ContentTable}}},
		{Content: []ContentBlock{{Kind: "unknown"}}},
	} {
		if err := New(Config{Transport: MethodHTTP, SMTP2GOAPIKey: "test", SMTP2GOEndpoint: server.URL}).Send(context.Background(), message); err == nil {
			t.Fatal("invalid content was accepted")
		}
	}
	select {
	case <-requests:
		t.Fatal("invalid content started an HTTP request")
	default:
	}
}
