package mailer

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCollectSysInfo_nonEmptyHostname(t *testing.T) {
	t.Parallel()
	si := CollectSysInfo(context.Background())
	if si.Hostname == "" {
		t.Fatal("expected hostname")
	}
}

func TestFirstHTTPBody_usesIPv4(t *testing.T) {
	t.Parallel()
	server := newFamilyHTTPServer(t, "tcp4", "127.0.0.1:0", "IPv4 ISP")

	got := firstHTTPBody(
		context.Background(),
		dialNetworkV4,
		[]string{server.URL},
		time.Second,
	)
	if got != "IPv4 ISP" {
		t.Fatalf("ISP = %q, want IPv4 ISP", got)
	}
}

func TestFirstHTTPBody_usesIPv6(t *testing.T) {
	t.Parallel()
	server := newFamilyHTTPServer(t, "tcp6", "[::1]:0", "IPv6 ISP")

	got := firstHTTPBody(
		context.Background(),
		dialNetworkV6,
		[]string{server.URL},
		time.Second,
	)
	if got != "IPv6 ISP" {
		t.Fatalf("ISP = %q, want IPv6 ISP", got)
	}
}

func newFamilyHTTPServer(t *testing.T, network, address, body string) *httptest.Server {
	t.Helper()
	listener, err := net.Listen(network, address)
	if err != nil {
		t.Fatalf("listen on %s: %v", network, err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(body))
	}))
	if err := server.Listener.Close(); err != nil {
		t.Fatalf("close default listener: %v", err)
	}
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return server
}
