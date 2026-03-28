package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const smtp2goSendURL = "https://api.smtp2go.com/v3/email/send"

type smtp2goPayload struct {
	APIKey        string          `json:"api_key"`
	Sender        string          `json:"sender"`
	To            []string        `json:"to"`
	Subject       string          `json:"subject"`
	TextBody      string          `json:"text_body"`
	HTMLBody      string          `json:"html_body,omitempty"`
	CustomHeaders []smtp2goHeader `json:"custom_headers,omitempty"`
}

type smtp2goHeader struct {
	Header string `json:"header"`
	Value  string `json:"value"`
}

type smtp2goResponse struct {
	Data struct {
		Succeeded int    `json:"succeeded"`
		Error     string `json:"error"`
	} `json:"data"`
}

func sendSMTP2GOHTTP(
	ctx context.Context,
	apiKey, from, to, subject, textBody, htmlBody, senderName string,
	bindIface string,
) error {
	payload := smtp2goPayload{
		APIKey:   apiKey,
		Sender:   from,
		To:       []string{to},
		Subject:  subject,
		TextBody: textBody,
		HTMLBody: htmlBody,
	}
	if senderName != "" {
		payload.CustomHeaders = []smtp2goHeader{
			{Header: "X-Sender-Name", Value: senderName},
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(
		cctx, http.MethodPost, smtp2goSendURL, bytes.NewReader(raw),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	if bindIface != "" {
		localIP, err := firstIPOnInterface(bindIface)
		if err != nil {
			return fmt.Errorf("bind interface %q: %w", bindIface, err)
		}
		d := &net.Dialer{Timeout: 10 * time.Second, LocalAddr: &net.TCPAddr{IP: localIP}}
		client.Transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return d.DialContext(ctx, "tcp", addr)
			},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("smtp2go http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed smtp2goResponse
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Data.Succeeded < 1 && parsed.Data.Error != "" {
			return fmt.Errorf("smtp2go api: %s", parsed.Data.Error)
		}
	}
	return nil
}

func firstIPOnInterface(name string) (net.IP, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if ip.To4() != nil {
			return ip.To4(), nil
		}
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip != nil && ip.To4() == nil && !ip.IsLoopback() {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("no usable IP on %q", name)
}
