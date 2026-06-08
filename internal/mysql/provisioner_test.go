package mysql

import "testing"

func TestEscapeString(t *testing.T) {
	cases := map[string]string{
		"simple":     "simple",
		"with'quote": `with\'quote`,
		`back\slash`: `back\\slash`,
		"a'b\\c":     `a\'b\\c`,
	}
	for in, want := range cases {
		if got := escapeString(in); got != want {
			t.Errorf("escapeString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateIdent(t *testing.T) {
	good := []string{"wp_blog", "wpu_shop_foo", "a1_2"}
	for _, s := range good {
		if err := validateIdent(s); err != nil {
			t.Errorf("validateIdent(%q) unexpected error: %v", s, err)
		}
	}
	bad := []string{"", "has space", "drop;table", "back`tick", "quote'd"}
	for _, s := range bad {
		if err := validateIdent(s); err == nil {
			t.Errorf("validateIdent(%q) should have failed", s)
		}
	}
}
