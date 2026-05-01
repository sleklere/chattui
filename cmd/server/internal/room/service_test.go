package room

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"uppercase", "TestString", "teststring"},
		{"space becomes hyphen", "Test Space", "test-space"},
		{"trims leading and trailing spaces", "  trim spaces  ", "trim-spaces"},
		{"multiple spaces become single hyphen", "multiple   spaces", "multiple-spaces"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := slugify(tc.input)
			if got != tc.expected {
				t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
