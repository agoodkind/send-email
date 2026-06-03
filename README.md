# send-email

Go CLI and library for sending email through **msmtp-compatible SMTP** (default) or the **SMTP2GO HTTP API**. It replaces the bash `send-email` helper used on homelab hosts and deploys to `/opt/scripts/send-email`.

## Requirements

- Go 1.22 or newer (to build)

## Build

```bash
make build
```

`make build` refreshes the shared `go-makefile` include at parse time, runs the shared non-test build checks, and then builds `./send-email`.

## Install

```bash
./install.sh
```

This compiles the binary and installs it to `/opt/scripts/send-email` (uses `sudo` when the directory is not writable).

## Usage

```text
send-email -t TO -s SUBJECT -m MSG [OPTIONS]
```

- `-t`: Recipient
- `-s`: Subject
- `-m`: Body (`\n` in the string becomes a newline)
- `-f`: From address (default: `$(hostname)-mailer@goodkind.io`)
- `-n`: Sender display name (default: hostname)
- `-c`: Caller label in metadata (default: `send-email`)
- `--http`: Force SMTP2GO HTTP API
- `-k`: SMTP2GO API key (optional if `SMTP2GO_API_KEY` env or env files below)
- `-i`: Outbound interface name (HTTP mode only)

API key resolution for HTTP mode: `-k`, then `SMTP2GO_API_KEY` in the environment, then `/etc/mwan-watchdog/watchdog.env`, then `/etc/mwan/mwan.env`.

## Library

Import `github.com/agoodkind/send-email/mailer` and use `mailer.New`, `Mailer.Send`, and helpers such as `mailer.ParseMsmtprc` / `mailer.LoadMsmtprc` if you need lower-level access.

## Tests

```bash
make test
```
