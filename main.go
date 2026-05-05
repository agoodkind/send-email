// Command send-email is a Go port of the bash send-email script.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/agoodkind/send-email/mailer"
)

const usageText = `Usage: %s -t TO -s SUBJECT -m MSG [OPTIONS]

Required:
  -t TO         Recipient email address
  -s SUBJECT    Email subject
  -m MSG        Email message body

Optional:
  -f FROM       Sender email (default: HOSTNAME-mailer@goodkind.io)
  -n NAME       Sender name (default: hostname)
  -c CALLER     Caller name for metadata (default: send-email)
  --http        Use SMTP2GO HTTP API instead of sendmail/msmtp
  -k API_KEY    SMTP2GO API key (optional if in env or env files)
  -i IFACE      Bind outbound HTTP to interface (HTTP mode only)
`

// errUsage is returned when CLI arguments are missing or invalid.
var errUsage = errors.New("invalid usage")

type cliArgs struct {
	to, subject, msg, from, name, caller, apiKey, bind string
	useHTTP                                            bool
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	slog.Info("send-email start")
	if err := run(context.Background(), os.Args); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintf(os.Stderr, usageText, os.Args[0])
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, argv []string) error {
	args, err := parseArgs(argv)
	if err != nil {
		return err
	}
	if args.apiKey == "" {
		args.apiKey = mailer.LoadAPIKeyFromEnvFiles([]string{
			"/etc/mwan-watchdog/watchdog.env",
			"/etc/mwan/mwan.env",
		})
	}
	cfg := mailer.Config{
		SMTP2GOAPIKey:     args.apiKey,
		MsmtprcPath:       "",
		DefaultFromDomain: "",
		BindInterface:     args.bind,
		Transport:         mailer.MethodAuto,
		Now:               nil,
	}
	if args.useHTTP {
		cfg.Transport = mailer.MethodHTTP
	}
	m := mailer.New(cfg)
	if err := m.Send(ctx, mailer.Message{
		To:      args.to,
		Subject: args.subject,
		Body:    args.msg,
		From:    args.from,
		Name:    args.name,
		Caller:  args.caller,
	}); err != nil {
		wrapped := fmt.Errorf("send: %w", err)
		slog.ErrorContext(ctx, "send-email failed", "err", wrapped)
		return wrapped
	}
	fmt.Printf("Email sent to %s via %s\n", args.to, m.Method())
	return nil
}

func parseArgs(argv []string) (cliArgs, error) {
	raw := argv[1:]
	useHTTP := slices.Contains(raw, "--http")
	filtered := slices.DeleteFunc(slices.Clone(raw), func(s string) bool {
		return s == "--http"
	})

	var args cliArgs
	args.useHTTP = useHTTP
	fs := flag.NewFlagSet(argv[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&args.to, "t", "", "")
	fs.StringVar(&args.subject, "s", "", "")
	fs.StringVar(&args.msg, "m", "", "")
	fs.StringVar(&args.from, "f", "", "")
	fs.StringVar(&args.name, "n", "", "")
	fs.StringVar(&args.caller, "c", "", "")
	fs.StringVar(&args.apiKey, "k", "", "")
	fs.StringVar(&args.bind, "i", "", "")
	if err := fs.Parse(filtered); err != nil {
		return cliArgs{}, fmt.Errorf("parse flags: %w", errUsage)
	}
	if fs.NArg() != 0 {
		return cliArgs{}, errUsage
	}
	if args.to == "" || args.subject == "" || args.msg == "" {
		return cliArgs{}, errUsage
	}
	return args, nil
}
