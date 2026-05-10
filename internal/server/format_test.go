package server

import (
	"testing"
	"time"
)

func TestFmtUTC(t *testing.T) {
	in := time.Date(2026, 5, 10, 14, 30, 45, 123456789, time.UTC)
	display, iso := fmtUTC(in)
	if want := "2026-05-10 14:30 UTC"; display != want {
		t.Errorf("display: got %q, want %q", display, want)
	}
	if want := "2026-05-10T14:30:45.123456789Z"; iso != want {
		t.Errorf("iso: got %q, want %q", iso, want)
	}
}
