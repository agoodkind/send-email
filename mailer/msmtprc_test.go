package mailer

import (
	"strings"
	"testing"
)

func TestParseMsmtprc_prepGuestsShape(t *testing.T) {
	t.Parallel()
	raw := `defaults
auth           on
tls            on
tls_starttls   on
tls_trust_file /etc/ssl/certs/ca-certificates.crt
logfile        /var/log/msmtp.log

account smtp2go
host mail.smtp2go.com
port 587
user host-mwan-home-goodkind-io
auth           login
password secret123
from mwan@goodkind.io

account default : smtp2go
`
	acc, err := ParseMsmtprc([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if acc.Host != "mail.smtp2go.com" {
		t.Fatalf("host: got %q", acc.Host)
	}
	if acc.Port != 587 {
		t.Fatalf("port: got %d", acc.Port)
	}
	if acc.User != "host-mwan-home-goodkind-io" {
		t.Fatalf("user: got %q", acc.User)
	}
	if acc.Password != "secret123" {
		t.Fatalf("password mismatch")
	}
	if acc.From != "mwan@goodkind.io" {
		t.Fatalf("from: got %q", acc.From)
	}
	if !acc.AuthLogin {
		t.Fatal("expected auth login")
	}
}

func TestParseMsmtprc_missingHost(t *testing.T) {
	t.Parallel()
	raw := `account smtp2go
user u
password p
from x@y.z

account default : smtp2go
`
	_, err := ParseMsmtprc([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected host error, got %v", err)
	}
}
