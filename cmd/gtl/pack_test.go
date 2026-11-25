package main

import "testing"

func TestEscapeTSVRoundTrip(t *testing.T) {
	for _, s := range []string{"a\nb", `c:\path\n`, "x\ty", "", "\\"} {
		if got := unescapeTSV(escapeTSV(s)); got != s {
			t.Errorf("%q → %q", s, got)
		}
	}
}
