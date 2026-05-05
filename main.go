// Command send-email is a Go port of the bash send-email script.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/agoodkind/send-email/mailer"
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s -t TO -s SUBJECT -m MSG [OPTIONS]

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
`, os.Args[0])
	os.Exit(1)
}

func main() {
	raw := os.Args[1:]
	useHTTP := slices.Contains(raw, "--http")
	filtered := slices.DeleteFunc(slices.Clone(raw), func(s string) bool {
		return s == "--http"
	})

	var to, subject, msg, from, name, caller, apiKey, bind string
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&to, "t", "", "")
	fs.StringVar(&subject, "s", "", "")
	fs.StringVar(&msg, "m", "", "")
	fs.StringVar(&from, "f", "", "")
	fs.StringVar(&name, "n", "", "")
	fs.StringVar(&caller, "c", "", "")
	fs.StringVar(&apiKey, "k", "", "")
	fs.StringVar(&bind, "i", "", "")
	if err := fs.Parse(filtered); err != nil {
		usage()
	}
	if fs.NArg() != 0 {
		usage()
	}

	if to == "" || subject == "" || msg == "" {
		usage()
	}

	if apiKey == "" {
		apiKey = mailer.LoadAPIKeyFromEnvFiles([]string{
			"/etc/mwan-watchdog/watchdog.env",
			"/etc/mwan/mwan.env",
		})
	}

	cfg := mailer.Config{
		SMTP2GOAPIKey: apiKey,
		BindInterface: bind,
	}
	if useHTTP {
		cfg.Transport = mailer.MethodHTTP
	} else {
		cfg.Transport = mailer.MethodAuto
	}

	m := mailer.New(cfg)
	ctx := context.Background()
	err := m.Send(ctx, mailer.Message{
		To:      to,
		Subject: subject,
		Body:    msg,
		From:    from,
		Name:    name,
		Caller:  caller,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Email sent to %s via %s\n", to, m.Method())
}
