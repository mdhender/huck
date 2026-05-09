# Huck Mailgun Lite Package

Status: implemented on 2026-05-09.

## Summary

Huck no longer depends on `github.com/mailgun/mailgun-go/v5`.
Mailgun delivery now goes through a tiny internal client in
`internal/email` that uses only the Go standard library.

This client is intentionally narrow. It supports only Huck's current
transactional-email need: sending a single HTML email through the
Mailgun `POST /v3/{domain}/messages` API.

## Package Layout

```text
internal/email/
    mailgun.go
    mailgun_test.go
```

`internal/mail` remains the application-facing package. Its
`MailgunMailer` now wraps `internal/email.Send`.

## Public API

```go
type Config struct {
    APIKey        string
    SendingDomain string
    FromAddress   string
    FromName      string
    BaseURL       string
    HTTPClient    *http.Client
}

func (c Config) Configured() bool

func Send(
    ctx context.Context,
    cfg Config,
    to string,
    subject string,
    htmlBody string,
) error
```

## Behavior

The client:

* uses `application/x-www-form-urlencoded`
* authenticates with Mailgun Basic Auth (`api` / API key)
* sends `from`, `to`, `subject`, and `html`
* preserves existing from-formatting behavior:
  * `"Name <email@example.com>"` when `FromName` is set
  * `"email@example.com"` otherwise
* respects the caller context
* does not create nested timeout contexts
* uses a reusable default `http.Client` with a 30-second timeout when
  no client is supplied
* treats non-2xx responses as errors and includes a small response-body
  snippet

Default base URL:

```text
https://api.mailgun.net
```

EU or test overrides can supply a different `BaseURL`. The client
appends `/v3/{domain}/messages` itself.

## Tests

The implementation is covered with `net/http/httptest` only.

Covered cases:

* `Config.Configured`
* from formatting with and without `FromName`
* successful send request shape
* HTTP 401 handling
* context cancellation
* invalid base URL
* `internal/mail.MailgunMailer` wrapper behavior

## Verification

Verified on 2026-05-09 with:

* `go build ./...`
* `go test ./...`
* `go vet ./...`
* `go mod graph | rg 'mailgun-go|chi'`

Live Mailgun verification was also performed with:

```text
go run ./cmd/sendtest
```

Using `.env.development.local`, Mailgun accepted the request and queued
the message, and the message was confirmed delivered end to end through
the Mailgun sandbox.

## Non-Goals

Still out of scope:

* attachments
* templates
* webhooks
* mailing lists
* event APIs
* tracking APIs
* batch sending
* retries/backoff
* MIME composition
* SDK parity with Mailgun

These should only be added if Huck grows a concrete need for them.
