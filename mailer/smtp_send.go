package mailer

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// dialTimeout bounds the TCP handshake when reaching the SMTP host.
const dialTimeout = 30 * time.Second

func buildMIMEMessage(
	fromDisplay, fromAddr, to, subject, boundary, textPart, htmlPart string,
	inlines []InlineImage,
) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s <%s>\r\n", fromDisplay, fromAddr)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	if len(inlines) == 0 {
		fmt.Fprintf(&b,
			"Content-Type: multipart/alternative; boundary=%q\r\n\r\n",
			boundary,
		)
		writeAlternative(&b, boundary, textPart, htmlPart)
		return []byte(b.String())
	}
	// Inline images ride in a related part beside the alternative one, so the
	// HTML can reach them through cid: while a text-only reader still gets the
	// plain part.
	related := boundary + "_rel"
	fmt.Fprintf(&b, "Content-Type: multipart/related; boundary=%q; type=%q\r\n\r\n", related, "multipart/alternative")
	fmt.Fprintf(&b, "--%s\r\n", related)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	writeAlternative(&b, boundary, textPart, htmlPart)
	for _, image := range inlines {
		fmt.Fprintf(&b, "--%s\r\n", related)
		fmt.Fprintf(&b, "Content-Type: %s\r\n", image.MIMEType)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-ID: <%s>\r\n", image.Filename)
		fmt.Fprintf(&b, "Content-Disposition: inline; filename=%q\r\n\r\n", image.Filename)
		b.WriteString(wrapBase64(image.Data))
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", related)
	return []byte(b.String())
}

func writeAlternative(b *strings.Builder, boundary, textPart, htmlPart string) {
	fmt.Fprintf(b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(textPart)
	b.WriteString("\r\n\r\n")
	fmt.Fprintf(b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	b.WriteString(htmlPart)
	b.WriteString("\r\n\r\n")
	fmt.Fprintf(b, "--%s--\r\n", boundary)
}

// wrapBase64 breaks the encoding into the 76 character lines RFC 2045 asks for.
func wrapBase64(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	var out strings.Builder
	for start := 0; start < len(encoded); start += base64LineLength {
		end := min(start+base64LineLength, len(encoded))
		out.WriteString(encoded[start:end])
		out.WriteString("\r\n")
	}
	return out.String()
}

func sendSMTPMSMTPCfg(
	ctx context.Context,
	acc Account,
	fromAddr, to string,
	msg []byte,
) error {
	addr := net.JoinHostPort(acc.Host, strconv.Itoa(acc.Port))
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		slog.ErrorContext(ctx, "smtp dial failed", "err", err, "addr", addr)
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, acc.Host)
	if err != nil {
		slog.ErrorContext(ctx, "smtp client failed", "err", err, "host", acc.Host)
		return fmt.Errorf("smtp client %s: %w", acc.Host, err)
	}
	defer func() { _ = client.Close() }()

	if acc.TLSStartTLS {
		tcfg := &tls.Config{ServerName: acc.Host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tcfg); err != nil {
			slog.ErrorContext(ctx, "smtp starttls failed", "err", err, "host", acc.Host)
			return fmt.Errorf("starttls: %w", err)
		}
	}
	auth := smtp.PlainAuth("", acc.User, acc.Password, acc.Host)
	if err := client.Auth(auth); err != nil {
		slog.ErrorContext(ctx, "smtp auth failed", "err", err, "host", acc.Host, "user", acc.User)
		return fmt.Errorf("auth: %w", err)
	}
	if err := client.Mail(fromAddr); err != nil {
		slog.ErrorContext(ctx, "smtp mail-from failed", "err", err, "from", fromAddr)
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		slog.ErrorContext(ctx, "smtp rcpt failed", "err", err, "to", to)
		return fmt.Errorf("rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		slog.ErrorContext(ctx, "smtp data open failed", "err", err)
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		slog.ErrorContext(ctx, "smtp data write failed", "err", err)
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		slog.ErrorContext(ctx, "smtp data close failed", "err", err)
		return fmt.Errorf("smtp close: %w", err)
	}
	if err := client.Quit(); err != nil {
		slog.ErrorContext(ctx, "smtp quit failed", "err", err)
		return fmt.Errorf("smtp quit: %w", err)
	}
	slog.InfoContext(ctx, "smtp send ok", "host", acc.Host, "to", to)
	return nil
}

// base64LineLength is the line width RFC 2045 sets for base64 bodies.
const base64LineLength = 76
