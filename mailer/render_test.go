package mailer

import (
	"context"
	"strings"
	"testing"
	"time"
)

// alertBodySample is the shape the watchdog and the OPNsense daemon send: a
// headline, a paragraph, then "Label: value" fields, one of which carries a
// raw command error that runs over several lines.
const alertBodySample = `Gateway rollback snapshots failing: disk pool pve/data is full

The watchdog has stopped taking known-good snapshots on this guest.

Action: Free space in pve/data (for example delete old snapshots) or grow it.
Alert_key: 113
Failed attempts: 3
Original error text: qm snapshot 113 known-good-20260914-231024: exit status 255: freeze guest filesystem
snapshotting drive-scsi0 (local-lvm:vm-113-disk-1)
thaw guest filesystem
snapshot create failed: starting cleanup
Transition: true
Note: <script>alert(1)</script>`

func fixedClock() Clock {
	return func() time.Time { return time.Date(2026, 9, 14, 23, 10, 24, 0, time.UTC) }
}

func TestRenderPlain(t *testing.T) {
	t.Parallel()
	got := RenderPlain(`line1\nline2`)
	if got != "line1\nline2" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatTextBody(t *testing.T) {
	t.Parallel()
	out := FormatTextBody("hello", "my-caller", "h1", nil)
	if !strings.Contains(out, "hello") {
		t.Fatal("missing body")
	}
	if !strings.Contains(out, "Caller: my-caller") {
		t.Fatal("missing caller")
	}
	if !strings.Contains(out, "Host: h1") {
		t.Fatal("missing host")
	}
	if !strings.Contains(out, "Time:") {
		t.Fatal("missing time")
	}
}

func TestRenderHTML_containsMetadata(t *testing.T) {
	t.Parallel()
	si := SysInfo{
		UptimeHuman:   "1d",
		LoadAverage:   "0.1 0.2 0.3",
		MemoryHuman:   "1G",
		DiskRootHuman: "10G free",
		PublicIPv4:    "1.2.3.4",
		PublicIPv6:    "2001:db8::1",
		ISP:           "ExampleISP",
	}
	html, err := RenderHTML("body\nline", "c1", "host1", si, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"body", "Caller", "c1", "host1", "Uptime", "1d", "ExampleISP"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q", want)
		}
	}
}

func TestRenderHTML_escapesBody(t *testing.T) {
	t.Parallel()
	si := CollectSysInfo(context.Background())
	html, err := RenderHTML("<script>x</script>", "c", "h", si, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatal("expected escaped script")
	}
}

// TestRenderHTML_alertBody drives the renderer with a realistic alert and
// checks that the headline, the fields, and the multi-line command error each
// land in their own part of the document.
func TestRenderHTML_alertBody(t *testing.T) {
	t.Parallel()
	html, err := RenderHTML(alertBodySample, "mwan-watchdog", "gw1", SysInfo{}, fixedClock())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(html, `<h1 class="headline">Gateway rollback snapshots failing: disk pool pve/data is full</h1>`) {
		t.Fatalf("headline not rendered as a heading:\n%s", html)
	}
	if !strings.Contains(html, `<p class="para">The watchdog has stopped taking known-good snapshots on this guest.</p>`) {
		t.Fatalf("paragraph not rendered as text:\n%s", html)
	}

	for label, value := range map[string]string{
		"Action":          "Free space in pve/data (for example delete old snapshots) or grow it.",
		"Alert_key":       "113",
		"Failed attempts": "3",
		"Transition":      "true",
	} {
		row := `<tr><th class="k" scope="row">` + label + `</th><td class="v">` + value + `</td></tr>`
		if !strings.Contains(html, row) {
			t.Fatalf("field %q missing its table row %q:\n%s", label, row, html)
		}
	}

	if !strings.Contains(html, `<th class="k wide" colspan="2" scope="row">Original error text</th>`) {
		t.Fatalf("multi-line field label not rendered above its value:\n%s", html)
	}
	pre := preBlock(t, html)
	for _, want := range []string{
		"qm snapshot 113 known-good-20260914-231024: exit status 255: freeze guest filesystem",
		"snapshotting drive-scsi0 (local-lvm:vm-113-disk-1)",
		"thaw guest filesystem",
		"snapshot create failed: starting cleanup",
	} {
		if !strings.Contains(pre, want) {
			t.Fatalf("preformatted block missing %q:\n%s", want, pre)
		}
	}
	if strings.Contains(pre, "Transition") {
		t.Fatalf("field after the multi-line value swallowed into it:\n%s", pre)
	}
}

// TestRenderHTML_escapesFieldValue keeps caller-supplied markup inert: an
// error string is untrusted input and must never reach the document as HTML.
func TestRenderHTML_escapesFieldValue(t *testing.T) {
	t.Parallel()
	html, err := RenderHTML(alertBodySample, "mwan-watchdog", "gw1", SysInfo{}, fixedClock())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("field value reached the document unescaped:\n%s", html)
	}
	if !strings.Contains(html, `<td class="v">&lt;script&gt;alert(1)&lt;/script&gt;</td>`) {
		t.Fatalf("escaped field value missing:\n%s", html)
	}
}

// TestFormatTextBody_alertBodyUnchanged pins the plain-text part of the mail
// byte for byte, so the HTML work above cannot alter what plain-text readers
// receive.
func TestFormatTextBody_alertBodyUnchanged(t *testing.T) {
	t.Parallel()
	got := FormatTextBody(alertBodySample, "mwan-watchdog", "gw1", fixedClock())
	want := alertBodySample + "\n\nCaller: mwan-watchdog\nHost: gw1\nTime: 2026-09-14 23:10:24 UTC"
	if got != want {
		t.Fatalf("plain body changed:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// preBlock returns the contents of the first <pre> element in html.
func preBlock(t *testing.T, html string) string {
	t.Helper()
	const open = `<pre class="out">`
	start := strings.Index(html, open)
	if start < 0 {
		t.Fatalf("no preformatted block in:\n%s", html)
	}
	rest := html[start+len(open):]
	end := strings.Index(rest, "</pre>")
	if end < 0 {
		t.Fatalf("unterminated preformatted block in:\n%s", html)
	}
	return rest[:end]
}
