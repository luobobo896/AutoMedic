package ws

import "testing"

func TestOriginAllowed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		origin, host string
		allow        []string
		want         bool
	}{
		{"", "example.com", nil, true},
		{"http://example.com", "example.com", nil, true},
		{"https://example.com", "example.com", nil, true},
		{"http://evil.example", "example.com", nil, false},
		{"http://localhost:5173", "127.0.0.1:8080", []string{"http://localhost:5173"}, true},
		{"http://evil.example", "127.0.0.1:8080", []string{"http://localhost:5173"}, false},
	}
	for _, tc := range cases {
		if got := originAllowed(tc.origin, tc.host, tc.allow); got != tc.want {
			t.Fatalf("originAllowed(%q,%q,%v)=%v want %v", tc.origin, tc.host, tc.allow, got, tc.want)
		}
	}
}
