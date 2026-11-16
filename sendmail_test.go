package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const sendmailTestMessage = `From: root
To: root
Subject: =?utf-8?b?dW5hdHRlbmRlZCB1cGdyYWRlcw==?=
MIME-Version: 1.0
Content-Type: text/plain; charset="utf-8"
Content-Transfer-Encoding: base64

cGFja2FnZXMgdXBncmFkZWQK
`

type recordedRequest struct {
	To       []string `json:"to"`
	Subject  string   `json:"subject"`
	Sender   string   `json:"sender"`
	TextBody string   `json:"text_body"`
}

func TestSendmailModeSendsParsedMessage(t *testing.T) {
	t.Parallel()

	var mutex sync.Mutex
	var recorded []recordedRequest
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			var payload recordedRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)

				return
			}

			mutex.Lock()
			recorded = append(recorded, payload)
			mutex.Unlock()

			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"data":{"succeeded":1}}`))
		}),
	)
	defer server.Close()

	binary := filepath.Join(t.TempDir(), "sendmail")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}

	var stderr bytes.Buffer
	command := exec.Command(binary, "-oi", "-t")
	command.Stdin = strings.NewReader(sendmailTestMessage)
	command.Stderr = &stderr
	command.Env = append(
		os.Environ(),
		"SMTP2GO_API_KEY=test-key",
		"SMTP2GO_ENDPOINT="+server.URL,
		"LOCAL_RECIPIENT=ops@example.invalid",
	)
	if err := command.Run(); err != nil {
		t.Fatalf("sendmail failed: %v\n%s", err, stderr.String())
	}

	mutex.Lock()
	defer mutex.Unlock()
	if len(recorded) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(recorded))
	}

	request := recorded[0]
	if len(request.To) != 1 || request.To[0] != "ops@example.invalid" {
		t.Fatalf("to = %v", request.To)
	}

	if request.Subject != "unattended upgrades" {
		t.Fatalf("subject = %q", request.Subject)
	}

	if !regexp.MustCompile(`^[A-Za-z0-9-]+-mailer@goodkind\.io$`).MatchString(request.Sender) {
		t.Fatalf("sender = %q", request.Sender)
	}

	if !strings.Contains(request.TextBody, "packages upgraded") {
		t.Fatalf("text_body = %q", request.TextBody)
	}
}
