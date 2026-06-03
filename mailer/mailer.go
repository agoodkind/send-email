// Package mailer sends mail via SMTP2GO HTTP or msmtp-compatible SMTP.
//
// The package exposes a [Mailer] that renders an HTML/text message and
// dispatches it through the configured [Transport].
package mailer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Transport selects how mail is sent when [MethodAuto].
type Transport string

const (
	// MethodAuto selects HTTP when an SMTP2GO key is present, otherwise
	// falls back to the local msmtp account.
	MethodAuto Transport = "auto"
	// MethodHTTP forces the SMTP2GO HTTP API transport.
	MethodHTTP Transport = "http"
	// MethodSendmail forces the msmtp-compatible SMTP transport.
	MethodSendmail Transport = "sendmail"
)

// Config configures Mailer construction.
type Config struct {
	SMTP2GOAPIKey string
	MsmtprcPath   string
	// DefaultFromDomain used when [Message.From] is empty
	// (e.g. hostname-mailer@goodkind.io).
	DefaultFromDomain string
	// BindInterface is optional; HTTP transport only (outbound interface name).
	BindInterface string
	Transport     Transport
	// Now overrides the clock used for timestamps. Defaults to [SystemClock].
	Now Clock
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

// New builds a [Mailer] from cfg. Empty path/key fields are resolved lazily.
func New(cfg Config) *Mailer {
	if cfg.DefaultFromDomain == "" {
		cfg.DefaultFromDomain = "goodkind.io"
	}
	if cfg.Transport == "" {
		cfg.Transport = MethodAuto
	}
	if cfg.Now == nil {
		cfg.Now = SystemClock
	}
	return &Mailer{cfg: cfg}
}

// Send renders rich HTML/text and delivers using the configured transport.
func (m *Mailer) Send(ctx context.Context, msg Message) error {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	from, name, caller := m.resolveIdentity(msg, host)

	si := CollectSysInfo(ctx)
	textBody := FormatTextBody(msg.Body, caller, host, m.cfg.Now)
	htmlBody, err := RenderHTML(msg.Body, caller, host, si, m.cfg.Now)
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	method := m.resolveMethod()
	slog.InfoContext(ctx, "send-email dispatch",
		"transport", string(method),
		"from", from,
		"to", msg.To,
		"caller", caller)
	switch method {
	case MethodHTTP:
		return m.sendHTTP(ctx, from, name, msg, textBody, htmlBody)
	case MethodSendmail:
		return m.sendSMTP(ctx, from, name, msg, textBody, htmlBody)
	case MethodAuto:
		// Auto resolves to HTTP or Sendmail above; reaching here is a bug.
		err := errors.New("auto transport not resolved")
		slog.ErrorContext(ctx, "send-email auto unresolved", "err", err)
		return err
	default:
		err := fmt.Errorf("unknown transport %q", method)
		slog.ErrorContext(ctx, "send-email unknown transport", "err", err)
		return err
	}
}

func (m *Mailer) resolveIdentity(msg Message, host string) (from, name, caller string) {
	from = strings.TrimSpace(msg.From)
	if from == "" {
		from = fmt.Sprintf("%s-mailer@%s", host, m.cfg.DefaultFromDomain)
	}
	name = strings.TrimSpace(msg.Name)
	if name == "" {
		name = host
	}
	caller = strings.TrimSpace(msg.Caller)
	if caller == "" {
		caller = "send-email"
	}
	return from, name, caller
}

func (m *Mailer) resolveMethod() Transport {
	method := m.cfg.Transport
	if method != MethodAuto {
		return method
	}
	if m.apiKey() != "" {
		return MethodHTTP
	}
	return MethodSendmail
}

func (m *Mailer) apiKey() string {
	key := strings.TrimSpace(m.cfg.SMTP2GOAPIKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("SMTP2GO_API_KEY"))
	}
	return key
}

func (m *Mailer) sendHTTP(
	ctx context.Context,
	from, name string,
	msg Message,
	textBody, htmlBody string,
) error {
	key := m.apiKey()
	if key == "" {
		err := errors.New("SMTP2GO_API_KEY required for HTTP transport")
		slog.ErrorContext(ctx, "send-email missing api key", "err", err)
		return err
	}
	bind := m.cfg.BindInterface
	return sendSMTP2GOHTTP(ctx, key, from, msg.To, msg.Subject, textBody, htmlBody, name, bind)
}

func (m *Mailer) sendSMTP(
	ctx context.Context,
	from, name string,
	msg Message,
	textBody, htmlBody string,
) error {
	path := m.cfg.MsmtprcPath
	if path == "" {
		path = "/etc/msmtprc"
	}
	acc, err := LoadMsmtprc(path)
	if err != nil {
		slog.ErrorContext(ctx, "send-email msmtprc load failed", "err", err, "path", path)
		return fmt.Errorf("msmtprc: %w", err)
	}
	boundary := fmt.Sprintf("----=_Part_%d_%d", m.cfg.Now().Unix(), os.Getpid())
	mime := buildMIMEMessage(name, from, msg.To, msg.Subject, boundary, textBody, htmlBody)
	return sendSMTPMSMTPCfg(ctx, acc, from, msg.To, mime)
}

// Method returns the resolved transport name after auto-detect (for logging).
func (m *Mailer) Method() string {
	if m.cfg.Transport != MethodAuto && m.cfg.Transport != "" {
		return string(m.cfg.Transport)
	}
	if m.apiKey() != "" {
		return string(MethodHTTP)
	}
	return string(MethodSendmail)
}

// LoadAPIKeyFromEnvFiles tries SMTP2GO_API_KEY from shell-style env files.
// It returns the first non-empty value found in paths.
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
		return "", errors.New("read env file")
	}
	prefix := key + "="
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			val := strings.TrimSpace(rest)
			val = strings.Trim(val, `"`)
			return val, nil
		}
	}
	return "", errors.New("key not found")
}

// ParseBoolEnv returns true if v is "1", "true", or "yes" (case insensitive).
func ParseBoolEnv(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes"
}

// AtoiDefault parses s as an int or returns def when s is empty or invalid.
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
