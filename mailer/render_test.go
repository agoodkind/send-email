package mailer

import (
	"context"
	"strings"
	"testing"
)

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
		ISPIPv4:       "Example IPv4 ISP",
		ISPIPv6:       "Example IPv6 ISP",
	}
	html, err := RenderHTML("body\nline", "c1", "host1", si, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"body",
		"Caller",
		"c1",
		"host1",
		"Uptime",
		"1d",
		"IPv4 ISP",
		"Example IPv4 ISP",
		"IPv6 ISP",
		"Example IPv6 ISP",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q", want)
		}
	}
}

func TestRenderHTML_keepsRawHTMLAndFooter(t *testing.T) {
	t.Parallel()
	si := SysInfo{
		UptimeHuman:   "2d",
		LoadAverage:   "0.4 0.5 0.6",
		MemoryHuman:   "2G",
		DiskRootHuman: "20G free",
		PublicIPv4:    "5.6.7.8",
		PublicIPv6:    "2001:db8::2",
		ISPIPv4:       "Other IPv4 ISP",
		ISPIPv6:       "Other IPv6 ISP",
	}
	raw := `<img src="cid:chart.png" alt="chart" width="600">`
	html, err := renderHTML("body", nil, raw, "c2", "host2", si, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, raw) {
		t.Fatalf("html dropped the raw block:\n%s", html)
	}
	// The metadata footer still has to survive beside the new block.
	for _, want := range []string{
		"Caller",
		"c2",
		"host2",
		"Uptime",
		"2d",
		"Other IPv4 ISP",
		"Other IPv6 ISP",
		"5.6.7.8",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing footer field %q", want)
		}
	}
}

func TestRenderHTML_usesLegacyISPForBothFamilies(t *testing.T) {
	t.Parallel()
	si := SysInfo{ISP: "Legacy ISP"}

	html, err := RenderHTML("body", "caller", "host", si, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<tr><td class="k">IPv4 ISP</td><td>Legacy ISP</td></tr>`,
		`<tr><td class="k">IPv6 ISP</td><td>Legacy ISP</td></tr>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered email missing %q", want)
		}
	}
}

func TestRenderHTML_escapesBody(t *testing.T) {
	t.Parallel()
	si := CollectSysInfo(context.Background())
	html, err := renderHTML("<script>x</script>", nil, "", "c", "h", si, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatal("expected escaped script")
	}
}
