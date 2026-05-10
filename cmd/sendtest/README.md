# sendtest

`sendtest` is Huck's Mailgun configuration validator. It sends a single
test email through the same `internal/mail.MailgunMailer` that
`huck serve` uses, so an operator can confirm that their Mailgun domain,
API key, `From:` address, and (optional) API base URL all work
end-to-end before bringing the server up.

It is **not** a throwaway: it is the documented smoke test for any
operator changing Mailgun credentials.

## When to use it

- Initial deployment: after populating `.env.development.local` (or the
  production equivalent), run `sendtest --to you@example.com` before
  running `huck serve` for the first time.
- After rotating the Mailgun API key, the sending domain, or the
  `From:` address.
- When swapping between Mailgun US (default) and EU
  (`--mailgun-api-base=https://api.eu.mailgun.net`).

## Installation

```sh
go install github.com/mdhender/huck/cmd/sendtest@latest
```

or, from a checkout of the repository:

```sh
CGO_ENABLED=0 go build -o sendtest ./cmd/sendtest
```

## Configuration

`sendtest` shares Huck's configuration plumbing: each flag is also an
`HUCK_*` environment variable, and values may come from a
`.env.{HUCK_ENV}.local` file loaded automatically by
`internal/dotenv`. The resolution order is the same as `cmd/huck`:
**CLI flag > environment variable > `--config` file > default**.

| Flag                  | Env var                  | Required | Notes                                                            |
| --------------------- | ------------------------ | -------- | ---------------------------------------------------------------- |
| `--mailgun-domain`    | `HUCK_MAILGUN_DOMAIN`    | yes      | The Mailgun sending domain.                                       |
| `--mailgun-api-key`   | `HUCK_MAILGUN_API_KEY`   | yes      | The Mailgun private API key.                                      |
| `--mailgun-from`      | `HUCK_MAILGUN_FROM`      | yes      | RFC 5322 `From:` string, e.g. `huck <noreply@example.com>`.       |
| `--mailgun-api-base`  | `HUCK_MAILGUN_API_BASE`  | no       | Empty → US default (`https://api.mailgun.net`). EU users set `https://api.eu.mailgun.net`. |
| `--to`                | `HUCK_TO`                | yes      | The recipient address for the test message.                       |
| `--config`            | —                        | no       | Optional path to a plain-format ff config file.                   |

`HUCK_ENV` defaults to `development`, so files matching
`.env.development.local`, `.env.local`, `.env.development`, and `.env`
are loaded (in that priority order) before flags are parsed.

## Examples

Using `.env.development.local`:

```sh
sendtest --to you@example.com
```

Overriding the domain on the CLI:

```sh
sendtest --mailgun-domain=mg.staging.example.com --to you@example.com
```

EU Mailgun account:

```sh
sendtest \
  --mailgun-api-base=https://api.eu.mailgun.net \
  --to you@example.com
```

## Expected output

**Success** — Mailgun accepted the message for delivery:

```
sending to you@example.com via domain mg.example.com ... queued
```

**Failure — missing flags:**

```
sendtest: missing required flag(s): [--mailgun-api-key]
```

**Failure — Mailgun rejected the request** (e.g. invalid API key,
sender not authorised, sandbox recipient not verified):

```
sending to you@example.com via domain mg.example.com ... FAIL
sendtest: mailgun: 401 Unauthorized: ...
```

In every failure case the binary exits with a non-zero status so it
can be wired into a deploy script.
