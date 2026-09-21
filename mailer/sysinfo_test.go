package mailer

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCollectSysInfo_nonEmptyHostname(t *testing.T) {
	t.Parallel()
	si := CollectSysInfo(context.Background())
	if si.Hostname == "" {
		t.Fatal("expected hostname")
	}
}

func TestCollectSysInfo_routesLookupsByIPFamily(t *testing.T) {
	baseURL := newDualStackHTTPServers(t)
	lookupURLs := networkLookupURLs{
		publicIP: []string{baseURL + "/ip"},
		isp:      []string{baseURL + "/isp"},
	}

	si := collectSysInfo(context.Background(), lookupURLs)

	if si.PublicIPv4 != "192.0.2.4" {
		t.Fatalf("PublicIPv4 = %q, want 192.0.2.4", si.PublicIPv4)
	}
	if si.PublicIPv6 != "2001:db8::6" {
		t.Fatalf("PublicIPv6 = %q, want 2001:db8::6", si.PublicIPv6)
	}
	if si.ISPIPv4 != "IPv4 ISP" {
		t.Fatalf("ISPIPv4 = %q, want IPv4 ISP", si.ISPIPv4)
	}
	if si.ISPIPv6 != "IPv6 ISP" {
		t.Fatalf("ISPIPv6 = %q, want IPv6 ISP", si.ISPIPv6)
	}
	html, err := RenderHTML("body", "test", "host", si, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<tr><td class="k">IPv4 ISP</td><td>IPv4 ISP</td></tr>`,
		`<tr><td class="k">IPv6 ISP</td><td>IPv6 ISP</td></tr>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("rendered email missing %q", want)
		}
	}
}

func TestCollectSysInfo_honorsCancellation(t *testing.T) {
	baseURL := newDualStackHTTPServers(t)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	lookupURLs := networkLookupURLs{
		publicIP: []string{baseURL + "/ip"},
		isp:      []string{baseURL + "/isp"},
	}

	si := collectSysInfo(parent, lookupURLs)

	if si.PublicIPv4 != "N/A" {
		t.Fatalf("PublicIPv4 = %q, want N/A", si.PublicIPv4)
	}
	if si.PublicIPv6 != "N/A" {
		t.Fatalf("PublicIPv6 = %q, want N/A", si.PublicIPv6)
	}
	if si.ISPIPv4 != "N/A" {
		t.Fatalf("ISPIPv4 = %q, want N/A", si.ISPIPv4)
	}
	if si.ISPIPv6 != "N/A" {
		t.Fatalf("ISPIPv6 = %q, want N/A", si.ISPIPv6)
	}
}

func newDualStackHTTPServers(t *testing.T) string {
	t.Helper()
	listener4, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on IPv4: %v", err)
	}
	tcpAddress, ok := listener4.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("IPv4 listener address has type %T", listener4.Addr())
	}
	listener6, err := net.Listen("tcp6", fmt.Sprintf("[::1]:%d", tcpAddress.Port))
	if err != nil {
		_ = listener4.Close()
		t.Fatalf("listen on IPv6: %v", err)
	}
	startFamilyHTTPServer(t, listener4, "192.0.2.4", "IPv4 ISP")
	startFamilyHTTPServer(t, listener6, "2001:db8::6", "IPv6 ISP")
	return fmt.Sprintf("http://localhost:%d", tcpAddress.Port)
}

func startFamilyHTTPServer(t *testing.T, listener net.Listener, publicIP, isp string) {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/ip":
			_, _ = writer.Write([]byte(publicIP))
		case "/isp":
			_, _ = writer.Write([]byte(isp))
		default:
			http.NotFound(writer, request)
		}
	}))
	if err := server.Listener.Close(); err != nil {
		t.Fatalf("close default listener: %v", err)
	}
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
}
