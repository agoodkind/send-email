package mailer

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Account holds parsed msmtp account fields used for SMTP submission.
type Account struct {
	Host           string
	Port           int
	User           string
	Password       string
	From           string
	AuthLogin      bool
	TLS            bool
	TLSStartTLS    bool
	TrustCertsFile string
}

type msmtpPartial struct {
	host, user, password, from string
	port                       int
	authLogin                  *bool
	tls, tlsStartTLS           *bool
	trustFile                  string
}

// msmtpFieldKey names the recognized account/defaults field keys in an
// msmtprc block.
type msmtpFieldKey string

const (
	fieldHost         msmtpFieldKey = "host"
	fieldPort         msmtpFieldKey = "port"
	fieldUser         msmtpFieldKey = "user"
	fieldPassword     msmtpFieldKey = "password"
	fieldFrom         msmtpFieldKey = "from"
	fieldAuth         msmtpFieldKey = "auth"
	fieldTLS          msmtpFieldKey = "tls"
	fieldTLSStartTLS  msmtpFieldKey = "tls_starttls"
	fieldTLSTrustFile msmtpFieldKey = "tls_trust_file"
)

func (m *msmtpPartial) applyLine(key string, parts []string) {
	if len(parts) < 2 {
		return
	}
	val := parts[1]
	switch msmtpFieldKey(key) {
	case fieldHost:
		m.host = val
	case fieldPort:
		if p, err := strconv.Atoi(val); err == nil {
			m.port = p
		}
	case fieldUser:
		m.user = val
	case fieldPassword:
		m.password = val
	case fieldFrom:
		m.from = val
	case fieldAuth:
		v := strings.ToLower(val) == "login"
		m.authLogin = &v
	case fieldTLS:
		v := strings.ToLower(val) == "on"
		m.tls = &v
	case fieldTLSStartTLS:
		v := strings.ToLower(val) == "on"
		m.tlsStartTLS = &v
	case fieldTLSTrustFile:
		m.trustFile = val
	}
}

func mergeMsmtp(defaults, acc msmtpPartial) Account {
	out := Account{
		Port:           587,
		TLS:            true,
		TLSStartTLS:    true,
		TrustCertsFile: "/etc/ssl/certs/ca-certificates.crt",
	}
	if defaults.trustFile != "" {
		out.TrustCertsFile = defaults.trustFile
	}
	if defaults.tls != nil {
		out.TLS = *defaults.tls
	}
	if defaults.tlsStartTLS != nil {
		out.TLSStartTLS = *defaults.tlsStartTLS
	}
	if defaults.authLogin != nil {
		out.AuthLogin = *defaults.authLogin
	}
	if defaults.port > 0 {
		out.Port = defaults.port
	}
	if acc.host != "" {
		out.Host = acc.host
	}
	if acc.port > 0 {
		out.Port = acc.port
	}
	if acc.user != "" {
		out.User = acc.user
	}
	if acc.password != "" {
		out.Password = acc.password
	}
	if acc.from != "" {
		out.From = acc.from
	}
	if acc.trustFile != "" {
		out.TrustCertsFile = acc.trustFile
	}
	if acc.tls != nil {
		out.TLS = *acc.tls
	}
	if acc.tlsStartTLS != nil {
		out.TLSStartTLS = *acc.tlsStartTLS
	}
	if acc.authLogin != nil {
		out.AuthLogin = *acc.authLogin
	}
	return out
}

// msmtpDirective names the top-level block directives in an msmtprc file.
type msmtpDirective string

const (
	directiveDefaults msmtpDirective = "defaults"
	directiveAccount  msmtpDirective = "account"
)

// ParseMsmtprc parses /etc/msmtprc-style content for the default account.
func ParseMsmtprc(data []byte) (Account, error) {
	var defaults msmtpPartial
	accounts := make(map[string]msmtpPartial)
	var defaultAlias string
	block := ""

	for raw := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		key := strings.ToLower(parts[0])
		switch msmtpDirective(key) {
		case directiveDefaults:
			block = "defaults"
		case directiveAccount:
			if len(parts) >= 4 && strings.ToLower(parts[1]) == "default" &&
				parts[2] == ":" {
				defaultAlias = parts[3]
				block = ""
				continue
			}
			if len(parts) >= 2 {
				block = parts[1]
			}
		default:
			if block == "defaults" {
				defaults.applyLine(key, parts)
			} else if block != "" {
				a := accounts[block]
				a.applyLine(key, parts)
				accounts[block] = a
			}
		}
	}

	if defaultAlias == "" {
		return Account{}, errors.New("msmtprc: missing 'account default : NAME'")
	}
	named, ok := accounts[defaultAlias]
	if !ok {
		return Account{}, fmt.Errorf("msmtprc: unknown account %q", defaultAlias)
	}
	acc := mergeMsmtp(defaults, named)
	if acc.Host == "" {
		return Account{}, errors.New("msmtprc: missing host")
	}
	if acc.From == "" {
		return Account{}, errors.New("msmtprc: missing from")
	}
	if acc.User == "" || acc.Password == "" {
		return Account{}, errors.New("msmtprc: missing user or password")
	}
	return acc, nil
}

// LoadMsmtprc reads and parses path (default /etc/msmtprc).
func LoadMsmtprc(path string) (Account, error) {
	if path == "" {
		path = "/etc/msmtprc"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		wrapped := fmt.Errorf("read msmtprc %q: %w", path, err)
		slog.Error("msmtprc read failed", "err", wrapped, "path", path)
		return Account{}, wrapped
	}
	return ParseMsmtprc(data)
}
