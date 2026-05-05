package mailer

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
)

//go:embed email.html
var emailHTMLTmpl string

// RenderPlain expands escape sequences in msg the same way echo -e would
// for simple "\n" pairs.
func RenderPlain(msg string) string {
	return strings.ReplaceAll(msg, `\n`, "\n")
}

type ipRow struct {
	Iface string
	IP    string
}

type htmlEmailData struct {
	Preheader string
	BodyLines []string
	Caller    string
	TimeStr   string
	Hostname  string
	Uptime    string
	Load      string
	Memory    string
	Disk      string
	Pub4      string
	Pub6      string
	ISP       string
	Local4    []ipRow
	Local6    []ipRow
}

// RenderHTML builds the multipart HTML body with a metadata footer.
//
// now defaults to [SystemClock] when nil; callers may inject a clock for
// deterministic tests.
func RenderHTML(msg, caller, hostname string, si SysInfo, now Clock) (string, error) {
	if now == nil {
		now = SystemClock
	}
	plainBody := RenderPlain(msg)
	oneLine := strings.ReplaceAll(strings.TrimSpace(plainBody), "\n", " ")
	bodyLines := strings.Split(plainBody, "\n")
	data := htmlEmailData{
		Preheader: oneLine,
		BodyLines: bodyLines,
		Caller:    caller,
		TimeStr:   now().Format("2006-01-02 15:04:05 MST"),
		Hostname:  hostname,
		Uptime:    si.UptimeHuman,
		Load:      si.LoadAverage,
		Memory:    si.MemoryHuman,
		Disk:      si.DiskRootHuman,
		Pub4:      si.PublicIPv4,
		Pub6:      si.PublicIPv6,
		ISP:       si.ISP,
		Local4:    parseIPRows(si.LocalIPv4Lines),
		Local6:    parseIPRows(si.LocalIPv6Lines),
	}
	tmpl, err := template.New("email").Parse(emailHTMLTmpl)
	if err != nil {
		wrapped := fmt.Errorf("parse email template: %w", err)
		slog.Error("email template parse failed", "err", wrapped)
		return "", wrapped
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		wrapped := fmt.Errorf("execute email template: %w", err)
		slog.Error("email template execute failed", "err", wrapped)
		return "", wrapped
	}
	return buf.String(), nil
}

func parseIPRows(lines []string) []ipRow {
	var rows []ipRow
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			rows = append(rows, ipRow{
				Iface: parts[0],
				IP:    strings.Join(parts[1:], " "),
			})
		}
	}
	return rows
}

// FormatTextBody appends a caller/host/time footer to the plain text body
// (matches the legacy bash send-email behavior).
//
// now defaults to [SystemClock] when nil.
func FormatTextBody(msg, caller, hostname string, now Clock) string {
	if now == nil {
		now = SystemClock
	}
	plain := RenderPlain(msg)
	return fmt.Sprintf(
		"%s\n\nCaller: %s\nHost: %s\nTime: %s",
		plain,
		caller,
		hostname,
		now().Format("2006-01-02 15:04:05 MST"),
	)
}
