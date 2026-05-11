package server

import (
	"strings"
	"testing"
)

// TestTopbarPartial pins the rendered shape of partials/topbar.html
// (Sprint 4 T1.4): the page title appears on the left, the signed-in
// handle and a POST /logout form appear on the right, and the form is
// plain (no class="inline") so .huck-topbar form can style it per T3.
func TestTopbarPartial(t *testing.T) {
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}

	v := TopbarView{Handle: "alice", Title: "Invites"}
	var buf strings.Builder
	if err := r.partials.ExecuteTemplate(&buf, "partials/topbar.html", v); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`class="huck-topbar"`,
		`Invites`,
		`alice`,
		`<form method="post" action="/logout">`,
		`type="submit"`,
		`Sign out`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}

	// The plan moves the .inline styling decision into T3 (.huck-topbar form),
	// so the partial must not hard-code class="inline" on the form.
	if strings.Contains(out, `class="inline"`) {
		t.Errorf("topbar form should not carry class=\"inline\"; styling lives in .huck-topbar form (T3)\n--- output ---\n%s", out)
	}

	// Title must appear before the handle so the layout is title-left,
	// user-right regardless of any CSS that might later reorder.
	titleIdx := strings.Index(out, "Invites")
	handleIdx := strings.Index(out, "alice")
	if titleIdx < 0 || handleIdx < 0 || titleIdx >= handleIdx {
		t.Errorf("title should appear before handle in source order: titleIdx=%d handleIdx=%d\n--- output ---\n%s",
			titleIdx, handleIdx, out)
	}
}
