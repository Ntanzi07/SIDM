package boxes

import (
	"strings"
	"testing"
)

func TestLeadingSHA1(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	cases := map[string]bool{
		sha + ".jpg":                  true,
		strings.ToUpper(sha) + ".jpg": true,
		sha + "_v2_8003_x.jpg":        true,
		sha:                           true,
		sha + "c.jpg":                 false, // 41 hex: não é um sha1
		"10_CHEVROLET_AGILE.jpg":      false,
		"zz" + sha[2:] + ".jpg":       false,
	}
	for name, want := range cases {
		if _, got := leadingSHA1(name); got != want {
			t.Errorf("leadingSHA1(%q) = %v, want %v", name, got, want)
		}
	}
}
