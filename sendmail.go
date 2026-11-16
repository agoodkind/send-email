package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"strings"

	"goodkind.io/send-email/mailer"
)

const (
	sendmailName       = "sendmail"
	localRecipientKey  = "LOCAL_RECIPIENT"
	localRecipientFile = "/etc/send-email/mail.env"
	smtp2goVariable    = "SMTP2GO_API_KEY"
	flagFrom           = "-f"
	flagReturn         = "-r"
	flagName           = "-F"
	textPlain          = "text/plain"
	multipartPrefix    = "multipart/"
	encodingBase64     = "base64"
	encodingQuoted     = "quoted-printable"
	transferEncoding   = "Content-Transfer-Encoding"
	contentType        = "Content-Type"
)

var (
	errNoRecipients        = errors.New("no recipients")
	errNoTextPart          = errors.New("message has no text/plain part")
	errLocalRecipientUnset = errors.New(localRecipientKey + " is not set")
)

type sendmailArgs struct {
	sender     string
	senderName string
	recipients []string
}

func runSendmail(ctx context.Context, argv []string, stdin io.Reader) error {
	args := parseSendmailArgs(argv)

	parsed, err := mail.ReadMessage(stdin)
	if err != nil {
		return fmt.Errorf("read message: %w", err)
	}

	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil {
		return fmt.Errorf("decode subject: %w", err)
	}

	body, err := decodeTextBody(
		parsed.Header.Get(contentType),
		parsed.Header.Get(transferEncoding),
		parsed.Body,
	)
	if err != nil {
		return err
	}

	recipients := args.recipients
	if len(recipients) == 0 {
		recipients = headerAddresses(parsed.Header.Get("To"))
	}

	if len(recipients) == 0 {
		return errNoRecipients
	}

	sender := args.sender
	if sender == "" {
		senders := headerAddresses(parsed.Header.Get("From"))
		if len(senders) > 0 {
			sender = senders[0]
		}
	}

	if !strings.Contains(sender, "@") {
		sender = ""
	}

	resolved, err := resolveLocalRecipients(recipients)
	if err != nil {
		return err
	}

	client := mailer.New(newConfig(sendmailAPIKey()))
	var failures []error

	for _, recipient := range resolved {
		sendErr := client.Send(
			ctx,
			mailer.Message{
				To:      recipient,
				Subject: subject,
				Body:    body,
				From:    sender,
				Name:    args.senderName,
				Caller:  sendmailName,
			},
		)
		if sendErr != nil {
			slog.ErrorContext(ctx, "sendmail failed", "err", sendErr, "to", recipient)
			failures = append(failures, fmt.Errorf("send to %s: %w", recipient, sendErr))
		}
	}

	return errors.Join(failures...)
}

func parseSendmailArgs(argv []string) sendmailArgs {
	var args sendmailArgs
	pending := ""

	for _, arg := range argv {
		if pending == flagName {
			args.senderName = arg
			pending = ""

			continue
		}

		if pending != "" {
			args.sender = arg
			pending = ""

			continue
		}

		if arg == flagFrom || arg == flagReturn || arg == flagName {
			pending = arg

			continue
		}

		if !strings.HasPrefix(arg, "-") {
			args.recipients = append(args.recipients, arg)
		}
	}

	return args
}

func headerAddresses(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	var addresses []string
	parsed, err := mail.ParseAddressList(trimmed)
	if err == nil {
		for _, address := range parsed {
			addresses = append(addresses, address.Address)
		}

		return addresses
	}

	for field := range strings.SplitSeq(trimmed, ",") {
		field = strings.TrimSpace(field)
		if field != "" {
			addresses = append(addresses, field)
		}
	}

	return addresses
}

func mediaTypeOf(header string) (string, map[string]string, error) {
	if strings.TrimSpace(header) == "" {
		return textPlain, nil, nil
	}

	mediaType, params, err := mime.ParseMediaType(header)
	if err != nil {
		wrapped := fmt.Errorf("parse %s %q: %w", contentType, header, err)
		slog.Error("sendmail media type rejected", "err", wrapped)

		return "", nil, wrapped
	}

	return mediaType, params, nil
}

func decodeTextBody(typeHeader, encoding string, body io.Reader) (string, error) {
	mediaType, params, err := mediaTypeOf(typeHeader)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(mediaType, multipartPrefix) {
		return readEncoded(encoding, body)
	}

	reader := multipart.NewReader(body, params["boundary"])

	for {
		part, partErr := reader.NextRawPart()
		if errors.Is(partErr, io.EOF) {
			return "", errNoTextPart
		}

		if partErr != nil {
			wrapped := fmt.Errorf("read multipart: %w", partErr)
			slog.Error("sendmail multipart read failed", "err", wrapped)

			return "", wrapped
		}

		partType, _, typeErr := mediaTypeOf(part.Header.Get(contentType))
		if typeErr != nil {
			return "", typeErr
		}

		if partType == textPlain {
			return readEncoded(part.Header.Get(transferEncoding), part)
		}
	}
}

func readEncoded(encoding string, body io.Reader) (string, error) {
	decoded := body
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case encodingBase64:
		decoded = base64.NewDecoder(base64.StdEncoding, body)
	case encodingQuoted:
		decoded = quotedprintable.NewReader(body)
	}

	data, err := io.ReadAll(decoded)
	if err != nil {
		wrapped := fmt.Errorf("decode body: %w", err)
		slog.Error("sendmail body decode failed", "err", wrapped)

		return "", wrapped
	}

	return string(data), nil
}

func resolveLocalRecipients(recipients []string) ([]string, error) {
	resolved := make([]string, 0, len(recipients))
	local := ""

	for _, recipient := range recipients {
		if strings.Contains(recipient, "@") {
			resolved = append(resolved, recipient)

			continue
		}

		if local == "" {
			local = lookupLocalRecipient()
		}

		if local == "" {
			wrapped := fmt.Errorf(
				"%w: set it in the environment or in %s",
				errLocalRecipientUnset,
				localRecipientFile,
			)
			slog.Error("sendmail local recipient missing", "err", wrapped)

			return nil, wrapped
		}

		resolved = append(resolved, local)
	}

	return resolved, nil
}

func lookupLocalRecipient() string {
	value := strings.TrimSpace(os.Getenv(localRecipientKey))
	if value != "" {
		return value
	}

	return readEnvFileValue(localRecipientFile, localRecipientKey)
}

func readEnvFileValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	prefix := key + "="

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}

	return ""
}

func sendmailAPIKey() string {
	if strings.TrimSpace(os.Getenv(smtp2goVariable)) != "" {
		return ""
	}

	return mailer.LoadAPIKeyFromEnvFiles(apiKeyFiles())
}
