// sendtest is Huck's Mailgun configuration validator. It sends one
// test email through the same internal/mail.MailgunMailer that
// `huck serve` uses, so an operator can prove that their Mailgun
// domain, API key, From: address, and (optional) API base URL are
// all valid before starting the server.
//
// Configuration follows the same flag/env/config-file path as
// `huck serve` (internal/config + ff.WithEnvVarPrefix("HUCK") +
// dotenv). The relevant inputs are:
//
//	--mailgun-domain    / HUCK_MAILGUN_DOMAIN     (required)
//	--mailgun-api-key   / HUCK_MAILGUN_API_KEY    (required)
//	--mailgun-from      / HUCK_MAILGUN_FROM       (required)
//	--mailgun-api-base  / HUCK_MAILGUN_API_BASE   (optional; EU users set
//	                                               https://api.eu.mailgun.net)
//	--to                / HUCK_TO                 (recipient address)
//	--config                                       (optional ff plain-format file)
//
// HUCK_ENV defaults to "development", so `.env.development.local` and
// `.env.development` are loaded automatically. See cmd/sendtest/README.md
// for usage examples and the expected success/failure output.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"

	"github.com/mdhender/huck/internal/config"
	"github.com/mdhender/huck/internal/dotenv"
	"github.com/mdhender/huck/internal/mail"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, ff.ErrHelp) || errors.Is(err, ff.ErrNoExec) {
			return
		}
		fmt.Fprintf(os.Stderr, "sendtest: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	env, ok := os.LookupEnv("HUCK_ENV")
	if !ok {
		env = "development"
	}
	if err := dotenv.Load(env); err != nil {
		return err
	}

	cfg := &config.Config{}
	var to string

	fs := ff.NewFlagSet("sendtest")
	fs.StringVar(&cfg.ConfigFile, 0, "config", "", "optional path to a plain-format config file")
	fs.StringVar(&cfg.MailgunDomain, 0, "mailgun-domain", "", "Mailgun sending domain")
	fs.StringVar(&cfg.MailgunAPIKey, 0, "mailgun-api-key", "", "Mailgun API key")
	fs.StringVar(&cfg.MailgunFrom, 0, "mailgun-from", "", "From: address (RFC 5322 string)")
	fs.StringVar(&cfg.MailgunAPIBase, 0, "mailgun-api-base", "", "Mailgun API base URL (without version suffix); empty = SDK default (US). EU example: https://api.eu.mailgun.net")
	fs.StringVar(&to, 0, "to", "", "recipient email address")

	if err := ff.Parse(fs, args,
		ff.WithEnvVarPrefix("HUCK"),
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ff.PlainParser),
		ff.WithConfigAllowMissingFile(),
	); err != nil {
		fmt.Fprintf(stderr, "%s\n", ffhelp.Flags(fs))
		return err
	}

	if err := cfg.ValidateMailer(); err != nil {
		return err
	}
	if to == "" {
		return errors.New("missing required flag(s): [--to]")
	}

	mg, err := mail.NewMailgunMailer(mail.MailgunConfig{
		Domain:  cfg.MailgunDomain,
		APIKey:  cfg.MailgunAPIKey,
		From:    cfg.MailgunFrom,
		APIBase: cfg.MailgunAPIBase,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	subject := "huck sendtest — " + time.Now().UTC().Format(time.RFC3339)
	body := `<!doctype html>
<html><body>
<p>Hello from huck's <code>internal/mail</code> package.</p>
<p>If you're reading this, the Mailgun credentials in your environment
work end-to-end against domain <code>` + cfg.MailgunDomain + `</code>.</p>
</body></html>`

	fmt.Fprintf(stdout, "sending to %s via domain %s ... ", to, cfg.MailgunDomain)
	if err := mg.Send(ctx, to, subject, body); err != nil {
		fmt.Fprintln(stdout, "FAIL")
		return err
	}
	fmt.Fprintln(stdout, "queued")
	return nil
}
