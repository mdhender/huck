// huck-specific frontend glue. Kept tiny so the strict default CSP
// (script-src 'self') applies cleanly without needing 'unsafe-inline'.

// Copy the _csrf cookie into the X-CSRF-Token header on every HTMX
// request so Echo's double-submit CSRF check accepts it.
document.body.addEventListener('htmx:configRequest', function (evt) {
    var match = document.cookie.match(/(?:^|;\s*)_csrf=([^;]+)/);
    if (match) {
        evt.detail.headers['X-CSRF-Token'] = decodeURIComponent(match[1]);
    }
});
