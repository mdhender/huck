// huck-specific frontend glue. Kept tiny so the strict default CSP
// (script-src 'self') applies cleanly without needing 'unsafe-inline'.
//
// CSRF / cross-origin request protection is now enforced server-side by
// net/http.CrossOriginProtection (see internal/server/middleware.go), so
// no per-request header mirroring is needed here.

// Rewrite <time datetime="..."> elements to render in the browser's
// local timezone. Server emits ISO-8601 UTC timestamps in the datetime
// attribute and a UTC fallback in the text content (so SSR / no-JS
// clients still see something sensible).
(function () {
    const fmt = new Intl.DateTimeFormat(undefined, {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
        timeZoneName: "short",
    });

    function localize(root) {
        const nodes = (root || document).querySelectorAll("time[datetime]");
        nodes.forEach(function (el) {
            if (el.dataset.huckLocalized === "1") return;
            const iso = el.getAttribute("datetime");
            const d = new Date(iso);
            if (isNaN(d.getTime())) return;
            el.textContent = fmt.format(d);
            el.title = iso;
            el.dataset.huckLocalized = "1";
        });
    }

    document.addEventListener("DOMContentLoaded", function () {
        localize(document);
    });

    // Re-run after HTMX swaps in new content.
    document.addEventListener("htmx:afterSettle", function (e) {
        localize(e.target);
    });
})();

// Confirm form submissions that opt in via `data-confirm="..."`. CSP-clean
// replacement for `onsubmit="return confirm(...)"` — inline event handlers
// are blocked by our `script-src 'self'` policy.
(function () {
    document.addEventListener("submit", function (e) {
        const form = e.target;
        if (!(form instanceof HTMLFormElement)) return;
        const msg = form.dataset.confirm;
        if (msg && !window.confirm(msg)) {
            e.preventDefault();
        }
    });
})();

// Copy-link buttons (sprint-5 T5.3). A button with `data-invite-url="…"`
// copies that URL to the clipboard and briefly flips its visible text to
// "Copied". Kept here (not Alpine) because our CSP is `script-src 'self'`
// — no `unsafe-eval`, which Alpine's directive evaluator needs.
(function () {
    const COPIED_MS = 1500;
    document.addEventListener("click", function (e) {
        const btn = e.target.closest("[data-invite-url]");
        if (!btn) return;
        e.preventDefault();
        const url = btn.dataset.inviteUrl;
        if (!url || !navigator.clipboard) return;
        navigator.clipboard.writeText(url).then(function () {
            if (btn.dataset.huckCopying === "1") return;
            const original = btn.textContent;
            btn.dataset.huckCopying = "1";
            btn.textContent = "Copied";
            window.setTimeout(function () {
                btn.textContent = original;
                delete btn.dataset.huckCopying;
            }, COPIED_MS);
        });
    });
})();
