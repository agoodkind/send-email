package mailer_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"goodkind.io/send-email/mailer"
)

func TestSendPreservesHTTPFailureDetails(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusBadGateway)
			if _, err := io.WriteString(writer, "upstream unavailable\n"); err != nil {
				t.Error(err)
			}
		}),
	)
	defer server.Close()
	client := mailer.New(mailer.Config{
		Transport:       mailer.MethodHTTP,
		SMTP2GOAPIKey:   "test",
		SMTP2GOEndpoint: server.URL,
	})
	err := client.Send(
		t.Context(),
		mailer.Message{
			To:      "recipient@example.invalid",
			Subject: "Failure details",
			Body:    "Test message",
		},
	)
	const expected = "smtp2go http 502: upstream unavailable"
	if err == nil || err.Error() != expected {
		t.Fatalf("HTTP failure = %v, want %q", err, expected)
	}
}
