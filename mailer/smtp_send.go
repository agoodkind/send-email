package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

func buildMIMEMessage(fromDisplay, fromAddr, to, subject, boundary, textPart, htmlPart string) []byte {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("From: %s <%s>\r\n", fromDisplay, fromAddr))
	b.WriteString(fmt.Sprintf("To: %s\r\n", to))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString(fmt.Sprintf(
		"Content-Type: multipart/alternative; boundary=%q\r\n\r\n",
		boundary,
	))
	b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(textPart)
	b.WriteString("\r\n\r\n")
	b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	b.WriteString(htmlPart)
	b.WriteString("\r\n\r\n")
	b.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	return []byte(b.String())
}

func sendSMTPMSMTPCfg(acc Account, fromDisplay, fromAddr, to, subject string, msg []byte) error {
	addr := net.JoinHostPort(acc.Host, strconv.Itoa(acc.Port))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, acc.Host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if acc.TLSStartTLS {
		tcfg := &tls.Config{ServerName: acc.Host, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tcfg); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	auth := smtp.PlainAuth("", acc.User, acc.Password, acc.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := client.Mail(fromAddr); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
