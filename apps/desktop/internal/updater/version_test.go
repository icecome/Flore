package updater

import "testing"

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.0.1", "0.0.1", 0},
		{"0.0.2", "0.0.1", 1},
		{"0.0.1-20260802", "0.0.1", 1},
		{"0.0.1-20260803", "0.0.1-20260802", 1},
		{"0.0.1-20260802", "0.0.1-20260803", -1},
		{"0.0.1-20260802", "0.0.1-20260802", 0},
		{"v0.0.1-20260803", "0.0.1-20260802", 1},
		{"1.0.0", "0.0.9", 1},
		{"0.1.0", "0.0.9", 1},
	}
	for _, c := range cases {
		got := CompareVersion(c.a, c.b)
		if got != c.want {
			t.Errorf("CompareVersion(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}
