# internal/email

This package holds Huck's tiny, stdlib-only Mailgun client.

## Why this exists

Huck originally used `github.com/mailgun/mailgun-go/v5` through
`internal/mail/mailgun.go`. That worked, but it pulled in a much larger
dependency tree than Huck needs for one narrow task: sending a single
HTML transactional email.

The project requirements for this replacement were:

* standard library only
* no Chi or other router dependency
* no full SDK surface area
* easy to read, test, and replace

So this package is effectively a small internal fork of the behavior
Huck relied on, not a general Mailgun SDK.

## What it does

Today it supports only:

* `POST /v3/{domain}/messages`
* Basic Auth with Mailgun API keys
* form fields for `from`, `to`, `subject`, and `html`

That is enough for Huck's current invite and transactional-email flows.

## What it does not do

This package intentionally does not handle:

* attachments
* templates
* webhooks
* mailing lists
* event APIs
* retries/backoff
* MIME composition
* broader Mailgun SDK compatibility

If Huck needs any of those later, prefer extending this package
deliberately rather than reintroducing a large dependency without first
updating the design docs.

## Relationship to `internal/mail`

`internal/email` is the low-level HTTP client.

`internal/mail` remains the application-facing package and adapter used
by the rest of Huck. That split lets handlers continue to depend on the
small `mail.Mailer` interface while this package stays focused on the
provider-specific HTTP call.
