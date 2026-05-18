// Package dbx holds small helpers shared by the typed SQLite stores
// (internal/users, internal/invites, and Sprint-6's internal/games).
// It deliberately does not depend on zombiezen.com/go/sqlite — these
// helpers are pure-Go conversions used at the column-decode and
// timestamp-write boundaries.
//
// Keep this package tiny. If a helper here needs SQLite types or a
// connection handle, it belongs in the store package instead.
package dbx

import "time"

// BoolToInt maps a Go bool to the 0/1 integer SQLite stores. The
// stores keep boolean columns as INTEGER (DESIGN.md §7.4) so this
// conversion is shared by every INSERT/UPDATE that touches a boolean
// column.
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ParseTime decodes an ISO-8601 timestamp written by NowISO (or by
// SQLite's strftime). An empty string or any parse failure returns
// the zero value — callers treat the zero time as "no timestamp"
// (e.g. user.IsSuspended() checks SuspendedAt.IsZero()).
func ParseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// NowISO returns the current UTC time formatted as ISO-8601 nanosecond
// precision — the canonical write format used by every store. Callers
// that also need the time.Time value should call time.Now().UTC()
// directly and format with time.RFC3339Nano.
func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
