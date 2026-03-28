package mailer

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strings"
	"time"
)

//go:embed email.html
var emailHTMLTmpl string

// RenderPlain expands escape sequences in msg the same way echo -e would for simple \n.
func RenderPlain(msg string) string {
	return strings.ReplaceAll(msg, `\n`, "\n")
}

type ipRow struct {
	Iface string
	IP    string
}

type htmlEmailData struct {
	Preheader    template.HTML
	PreheaderPad template.HTML
	BodyHTML     template.HTML
	Caller       string
	TimeStr      string
	Hostname     string
	Uptime       string
	Load         string
	Memory       string
	Disk         string
	Pub4         string
	Pub6         string
	ISP          string
	Local4       []ipRow
	Local6       []ipRow
}

// RenderHTML builds the multipart HTML body with metadata footer.
func RenderHTML(msg, caller, hostname string, si SysInfo) (string, error) {
	plainBody := RenderPlain(msg)
	oneLine := strings.ReplaceAll(strings.TrimSpace(plainBody), "\n", " ")
	pad := strings.Repeat("&nbsp;&zwnj;", 100)
	var local4 []ipRow
	for _, line := range si.LocalIPv4Lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			local4 = append(local4, ipRow{Iface: parts[0], IP: strings.Join(parts[1:], " ")})
		}
	}
	var local6 []ipRow
	for _, line := range si.LocalIPv6Lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			local6 = append(local6, ipRow{Iface: parts[0], IP: strings.Join(parts[1:], " ")})
		}
	}
	bodyHTML := template.HTML(strings.ReplaceAll(
		template.HTMLEscapeString(plainBody), "\n", "<br>\n"))
	data := htmlEmailData{
		Preheader:    template.HTML(template.HTMLEscapeString(oneLine)),
		PreheaderPad: template.HTML(pad),
		BodyHTML:     bodyHTML,
		Caller:       caller,
		TimeStr:      time.Now().Format("2006-01-02 15:04:05 MST"),
		Hostname:     hostname,
		Uptime:       si.UptimeHuman,
		Load:         si.LoadAverage,
		Memory:       si.MemoryHuman,
		Disk:         si.DiskRootHuman,
		Pub4:         si.PublicIPv4,
		Pub6:         si.PublicIPv6,
		ISP:          si.ISP,
		Local4:       local4,
		Local6:       local6,
	}
	tmpl, err := template.New("email").Parse(emailHTMLTmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FormatTextBody appends caller/host/time footer to plain text (bash send-email behavior).
func FormatTextBody(msg, caller, hostname string) string {
	plain := RenderPlain(msg)
	return fmt.Sprintf(
		"%s\n\nCaller: %s\nHost: %s\nTime: %s",
		plain,
		caller,
		hostname,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)
}
