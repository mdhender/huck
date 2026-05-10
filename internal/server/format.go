package server

import "time"

// fmtUTC formats t for the admin tables: a human-friendly UTC string
// for the visible cell and an ISO-8601 string for the <time datetime>
// attribute. Centralising the layout strings keeps every admin row in
// sync — if the display format changes, it changes here.
func fmtUTC(t time.Time) (display, iso string) {
	return t.Format("2006-01-02 15:04 UTC"), t.Format(time.RFC3339Nano)
}
