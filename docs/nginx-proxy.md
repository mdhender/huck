# Nginx Reverse Proxy Contract

Single source of truth for the Nginx configuration that fronts `huck
serve` in production. The Go listener speaks plaintext HTTP on the
loopback interface; Nginx terminates TLS and proxies to it. The
contract below is what Nginx must (and must not) do for the
application's security model to hold.

Background on each header lives in `docs/DESIGN.md` §8.5 (CSRF /
cross-origin protection) and §12.1 (production TLS). This document
restates the operational pieces in one place so a DevOps reader does
not have to follow cross-references.

## Topology

Note: assumes port 8080 for documentation purposes.

```
browser ──HTTPS──▶ Nginx (TLS terminator) ──HTTP──▶ huck serve (127.0.0.1:8080)
```

- TLS terminates at Nginx. The Go process never speaks TLS.
- Nginx and `huck serve` are colocated on the same host; the upstream
  connection is plaintext on loopback.
- Local development (`huck serve` direct, no Nginx) is **out of scope**.
  Dev runs on `localhost`, which browsers treat as a Secure Context, so
  cross-origin checks still function over plain HTTP at the cost of a
  slightly larger residual CSRF window.

## Required configuration

These directives are load-bearing. Missing any of them is a defect.

### 1. Preserve the `Host` header

```nginx
proxy_set_header Host $host;
```

`http.CrossOriginProtection` (Go 1.25+) decides whether a request is
same-origin by comparing the host portion of the `Origin` header
against `Request.Host`. Without `proxy_set_header Host $host;`, Nginx
defaults to the upstream address (`127.0.0.1:8080`) and every
legitimate browser POST is rejected with 403 because the browser's
`Origin` host (`huck.example.com`) no longer matches.

### 2. Do not strip `Origin` or `Sec-Fetch-Site`

Nginx forwards client headers transparently by default. These two in
particular must reach the Go process:

- `Origin` — set by browsers on cross-origin requests and (in modern
  browsers) on every POST.
- `Sec-Fetch-Site` — set by browsers on all requests since 2023.

Both have hyphens rather than underscores, so the
`underscores_in_headers` directive does not drop them. **Do not** add
`proxy_set_header Origin ""` or a WAF rule that filters Fetch metadata
headers — either will break cross-origin protection. No explicit
`proxy_set_header` is needed to forward them; the default behaviour is
correct.

### 3. Require TLS 1.3

```nginx
ssl_protocols TLSv1.3;
ssl_prefer_server_ciphers off;
```

Per Alex Edwards' analysis of `http.CrossOriginProtection`
(<https://www.alexedwards.net/blog/preventing-csrf-in-go>), the
middleware's residual CSRF window narrows to roughly "Firefox v60–69
plus a small set of non-major browsers" once TLS 1.3 is enforced.
Older clients without `Sec-Fetch-Site` / `Origin` enforcement are
refused at the TLS layer before they ever reach a handler.

If a deployment cannot pin TLS 1.3 (e.g. a legacy client requires
TLSv1.2), record the wider residual risk in the deployment runbook
and rely more heavily on `SameSite=Lax` + HSTS as described in
DESIGN.md §8.5.

Note: the project has decided not to support incompatible legacy clients. This section is a reminder of that decision.

## Recommended configuration

Not load-bearing for `CrossOriginProtection`, but make the request
logger and any future Secure-cookie autodetection behave correctly:

```nginx
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
```

HSTS is also set by Huck when `--cookie-secure` is true. Setting it at
Nginx in addition is harmless and keeps the security-header story
visible to a DevOps reader who is only looking at the proxy:

```nginx
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

## Full sample

A reasonable starting point for `/etc/nginx/sites-available/huck.conf`:

```nginx
# Redirect plaintext HTTP to HTTPS.
server {
    listen 80;
    listen [::]:80;
    server_name huck.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name huck.example.com;

    # Required — see "Require TLS 1.3".
    ssl_protocols TLSv1.3;
    ssl_prefer_server_ciphers off;

    ssl_certificate     /etc/letsencrypt/live/huck.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/huck.example.com/privkey.pem;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    location / {
        proxy_pass http://127.0.0.1:8080;

        # Required — see "Preserve the Host header".
        proxy_set_header Host $host;

        # Recommended.
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # HTTP/1.1 keep-alive between Nginx and the Go app.
        proxy_http_version 1.1;
        proxy_set_header   Connection "";
    }
}
```

On Nginx 1.25+ replace `listen 443 ssl http2;` with `listen 443 ssl;`
plus a top-level `http2 on;` inside the server block. The deprecated
form above still works and is portable across older releases.

## Operational checklist

Before a release goes to production, verify:

- `nginx -T | grep ssl_protocols` returns `ssl_protocols TLSv1.3;`
  with no `TLSv1.2` fallback.
- `nginx -T | grep -A1 "server_name huck"` shows
  `proxy_set_header Host $host;` in the Huck server block.
- No `proxy_set_header Origin` or `proxy_set_header Sec-Fetch-Site`
  directive overrides the client value with `""` or a static string,
  and no WAF rule strips them.
- A test POST from a different origin returns 403, e.g.

  ```sh
  curl -i -X POST \
       -H "Origin: https://attacker.example" \
       -H "Content-Type: application/x-www-form-urlencoded" \
       --data "handle=alice&password=hunter2" \
       https://huck.example.com/login
  ```
- A normal browser form submit (same-origin) succeeds.

## Closed questions

- Confirm the production Nginx config we plan to ship pins `ssl_protocols TLSv1.3;` with no TLSv1.2 fallback. Tracked in DESIGN.md §8.5; close before Sprint 3 ships.

Answer: the project has decided not to support incompatible legacy clients.
