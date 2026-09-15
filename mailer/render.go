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

// bodyField is one "Label: value" pair parsed out of the plain body.
// Value holds every line of the value; Multiline marks the ones that need a
// preformatted block rather than a table cell.
type bodyField struct {
	Label     string
	Value     string
	Multiline bool
}

// bodyBlock is one blank-line-separated block of the plain body. A block is
// either a run of fields or a paragraph of free text, never both.
type bodyBlock struct {
	Paragraph []string
	Fields    []bodyField
}

// parsedBody is the plain body split into the shape the HTML template
// renders: a headline above blocks of fields and paragraphs.
type parsedBody struct {
	Headline string
	Blocks   []bodyBlock
}

type htmlEmailData struct {
	Preheader string
	Headline  string
	Blocks    []bodyBlock
	Tables    []Table
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

// RenderHTML builds the multipart HTML body with a metadata footer. It reads
// the structure alert senders already put in the plain body (see [parseBody])
// and renders the headline as a heading, "Label: value" pairs as a table, and
// a multi-line value as a preformatted block.
//
// now defaults to [SystemClock] when nil; callers may inject a clock for
// deterministic tests.
func RenderHTML(msg, caller, hostname string, si SysInfo, now Clock) (string, error) {
	return renderHTML(msg, nil, caller, hostname, si, now)
}

func renderHTML(msg string, tables []Table, caller string, hostname string, si SysInfo, now Clock) (string, error) {
	if now == nil {
		now = SystemClock
	}
	plainBody := RenderPlain(msg)
	oneLine := strings.ReplaceAll(strings.TrimSpace(plainBody), "\n", " ")
	parsed := parseBody(plainBody)
	data := htmlEmailData{
		Preheader: oneLine,
		Headline:  parsed.Headline,
		Blocks:    parsed.Blocks,
		Tables:    tables,
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

// Label shape limits. A label is short and word-like; anything longer or
// odder is prose that happens to contain a colon.
const (
	fieldLabelMaxChars = 40
	fieldLabelMaxWords = 4
)

// parseBody recovers the structure alert senders put in the plain body: the
// first non-empty line is the headline, a "Label: value" line opens a field,
// a following line that is not itself a label continues that field's value, a
// blank line closes the block, and free text outside any field is a
// paragraph.
func parseBody(plain string) parsedBody {
	lines := strings.Split(plain, "\n")
	parser := &bodyParser{}
	rest := lines
	for index, line := range lines {
		if strings.TrimSpace(line) != "" {
			parser.out.Headline = strings.TrimSpace(line)
			rest = lines[index+1:]
			break
		}
	}
	for _, line := range rest {
		parser.addLine(line)
	}
	parser.closeBlock()
	return parser.out
}

// bodyParser accumulates one parsedBody line by line. An open field keeps
// collecting continuation lines until a label line or a blank line ends it.
type bodyParser struct {
	out       parsedBody
	block     bodyBlock
	label     string
	value     []string
	fieldOpen bool
}

func (p *bodyParser) addLine(line string) {
	if strings.TrimSpace(line) == "" {
		p.closeBlock()
		return
	}
	if label, value, ok := splitField(line); ok {
		p.closeField()
		if len(p.block.Paragraph) > 0 {
			p.closeBlock()
		}
		p.label = label
		p.value = nil
		if value != "" {
			p.value = []string{value}
		}
		p.fieldOpen = true
		return
	}
	if p.fieldOpen {
		p.value = append(p.value, strings.TrimRight(line, " \t"))
		return
	}
	p.block.Paragraph = append(p.block.Paragraph, strings.TrimSpace(line))
}

func (p *bodyParser) closeField() {
	if !p.fieldOpen {
		return
	}
	p.block.Fields = append(p.block.Fields, bodyField{
		Label:     p.label,
		Value:     strings.Join(p.value, "\n"),
		Multiline: len(p.value) > 1,
	})
	p.label = ""
	p.value = nil
	p.fieldOpen = false
}

func (p *bodyParser) closeBlock() {
	p.closeField()
	if len(p.block.Fields) > 0 || len(p.block.Paragraph) > 0 {
		p.out.Blocks = append(p.out.Blocks, p.block)
	}
	p.block = bodyBlock{}
}

// splitField reports whether line opens a field. Only label-shaped text
// before the first colon counts, so a wrapped command error such as
// "snapshot create failed: starting cleanup" continues the field above it
// instead of opening a new one.
func splitField(line string) (label, value string, ok bool) {
	trimmed := strings.TrimRight(line, " \t")
	colon := strings.Index(trimmed, ":")
	if colon <= 0 {
		return "", "", false
	}
	label = trimmed[:colon]
	if !isFieldLabel(label) {
		return "", "", false
	}
	return label, strings.TrimSpace(trimmed[colon+1:]), true
}

// isFieldLabel reports whether label looks like an alert field name. Senders
// capitalize their keys, so an upper-case first letter separates a real label
// from a lower-case sentence fragment inside a multi-line value.
func isFieldLabel(label string) bool {
	if label == "" || len(label) > fieldLabelMaxChars {
		return false
	}
	if label[0] < 'A' || label[0] > 'Z' {
		return false
	}
	if strings.HasSuffix(label, " ") {
		return false
	}
	words := 1
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		case r == ' ':
			words++
		default:
			return false
		}
	}
	return words <= fieldLabelMaxWords
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

func formatTextTables(body string, tables []Table) string {
	lines := []string{body}
	for _, table := range tables {
		lines = append(lines, "")
		if table.Caption != "" {
			lines = append(lines, table.Caption)
		}
		if len(table.Headers) > 0 {
			lines = append(lines, strings.Join(table.Headers, " | "))
		}
		for _, row := range table.Rows {
			lines = append(lines, strings.Join(row, " | "))
		}
	}
	return strings.Join(lines, "\n")
}
