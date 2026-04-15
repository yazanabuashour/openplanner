package app

import "testing"

func TestBanner(t *testing.T) {
	t.Parallel()

	const want = "openplanner: Spec-first local planning SDK."

	if got := Banner(); got != want {
		t.Fatalf("Banner() = %q, want %q", got, want)
	}
}
