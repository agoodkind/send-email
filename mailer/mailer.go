package mailer

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Transport selects how mail is sent when MethodAuto.
type Transport string

const (
	MethodAuto     Transport = "auto"
	MethodHTTP     Transport = "http"
	MethodSendmail Transport = "sendmail"
)

// Config configures Mailer construction.
type Config struct {
	SMTP2GOAPIKey string
	MsmtprcPath   string
	// DefaultFrom used when Message.From is empty (e.g. hostname-mailer@goodkind.io).
	DefaultFromDomain string
	// BindInterface is optional; HTTP transport only (outbound interface name).
	BindInterface string
	Transport     Transport
}

// Message is one outbound email.
type Message struct {
	To      string
	Subject string
	Body    string
	From    string
	Name    string
	Caller  string
}

// Mailer sends email via SMTP2GO HTTP or msmtp-compatible SMTP.
type Mailer struct {
	cfg Config
}

// New builds a Mailer from config (paths and keys may be empty for lazy resolution).
func New(cfg Config) *Mailer {
	if cfg.DefaultFromDomain == "" {
		cfg.DefaultFromDomain = "goodkind.io"
	}
	if cfg.Transport == "" {
		cfg.Transport = MethodAuto
	}
	return &Mailer{cfg: cfg}
}

// Send renders rich HTML/text and delivers using configured transport.
func (m *Mailer) Send(ctx context.Context, msg Message) error {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	from := strings.TrimSpace(msg.From)
	if from == "" {
		from = fmt.Sprintf("%s-mailer@%s", host, m.cfg.DefaultFromDomain)
	}
	name := strings.TrimSpace(msg.Name)
	if name == "" {
		name = host
	}
	caller := strings.TrimSpace(msg.Caller)
	if caller == "" {
		caller = "send-email"
	}

	si := CollectSysInfo(ctx)
	textBody := FormatTextBody(msg.Body, caller, host)
	htmlBody, err := RenderHTML(msg.Body, caller, host, si)
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	method := m.cfg.Transport
	if method == MethodAuto {
		key := strings.TrimSpace(m.cfg.SMTP2GOAPIKey)
		if key == "" {
			key = strings.TrimSpace(os.Getenv("SMTP2GO_API_KEY"))
		}
		if key != "" {
			method = MethodHTTP
		} else {
			method = MethodSendmail
		}
	}

	switch method {
	case MethodHTTP:
		key := strings.TrimSpace(m.cfg.SMTP2GOAPIKey)
		if key == "" {
			key = strings.TrimSpace(os.Getenv("SMTP2GO_API_KEY"))
		}
		if key == "" {
			return fmt.Errorf("SMTP2GO_API_KEY required for HTTP transport")
		}
		bind := m.cfg.BindInterface
		return sendSMTP2GOHTTP(ctx, key, from, msg.To, msg.Subject, textBody, htmlBody, name, bind)
	case MethodSendmail:
		path := m.cfg.MsmtprcPath
		if path == "" {
			path = "/etc/msmtprc"
		}
		acc, err := LoadMsmtprc(path)
		if err != nil {
			return fmt.Errorf("msmtprc: %w", err)
		}
		boundary := fmt.Sprintf("----=_Part_%d_%d", time.Now().Unix(), os.Getpid())
		mime := buildMIMEMessage(name, from, msg.To, msg.Subject, boundary, textBody, htmlBody)
		return sendSMTPMSMTPCfg(acc, name, from, msg.To, msg.Subject, mime)
	default:
		return fmt.Errorf("unknown transport %q", method)
	}
}

// Method returns the resolved transport name after auto-detect (for logging).
func (m *Mailer) Method() string {
	if m.cfg.Transport != MethodAuto && m.cfg.Transport != "" {
		return string(m.cfg.Transport)
	}
	if strings.TrimSpace(m.cfg.SMTP2GOAPIKey) != "" ||
		strings.TrimSpace(os.Getenv("SMTP2GO_API_KEY")) != "" {
		return string(MethodHTTP)
	}
	return string(MethodSendmail)
}

// LoadAPIKeyFromEnvFiles tries SMTP2GO_API_KEY from shell-style env files (bash send-email).
func LoadAPIKeyFromEnvFiles(paths []string) string {
	for _, p := range paths {
		k, err := parseEnvFileKey(p, "SMTP2GO_API_KEY")
		if err == nil && k != "" {
			return k
		}
	}
	return ""
}

func parseEnvFileKey(path, key string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			val := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			val = strings.Trim(val, `"`)
			return val, nil
		}
	}
	return "", fmt.Errorf("key not found")
}

// ParseBoolEnv returns true if v is 1, true, yes (case insensitive).
func ParseBoolEnv(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes"
}

// AtoiDefault parses int or returns default.
func AtoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
