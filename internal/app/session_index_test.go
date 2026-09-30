package app

import "testing"

func TestSessionIndexSuffix(t *testing.T) {
	cases := map[string]string{
		"session-0":      " [0]",
		"session-12":     " [12]",
		"build-session3": " [3]",
		"local":          "",
		"session-":       "",
		"":               "",
	}
	for name, want := range cases {
		if got := sessionIndexSuffix(name); got != want {
			t.Errorf("sessionIndexSuffix(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestWithSessionIndex(t *testing.T) {
	cases := []struct {
		name, title, want string
	}{
		{"session-1", "work", "work [1]"},
		{"session-1", "work [1]", "work [1]"},
		{"session-1", "session-1", "session-1"},
		{"session-1", "~/tmp", "~/tmp [1]"},
		{"session-1", "", ""},
		{"local", "work", "work"},
	}
	for _, c := range cases {
		if got := withSessionIndex(c.name, c.title); got != c.want {
			t.Errorf("withSessionIndex(%q, %q) = %q, want %q", c.name, c.title, got, c.want)
		}
	}
}
