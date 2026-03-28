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
	out := FormatTextBody("hello", "my-caller", "h1")
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
	html, err := RenderHTML("body\nline", "c1", "host1", si)
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
	html, err := RenderHTML("<script>x</script>", "c", "h", si)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatal("expected escaped script")
	}
}
