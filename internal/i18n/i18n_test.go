package i18n

import "testing"

func TestT(t *testing.T) {
	Register(map[string]string{"здравей %s": "hello %s"})
	defer SetHub("bg")
	if got := Tf("здравей %s", "X"); got != "здравей X" {
		t.Fatal(got)
	}
	SetHub("en")
	if got := Tf("здравей %s", "X"); got != "hello X" {
		t.Fatal(got)
	}
	if got := T("няма превод"); got != "няма превод" {
		t.Fatal(got)
	}
	SetHub("de")
	if Hub() != "bg" {
		t.Fatal(Hub())
	}
}
