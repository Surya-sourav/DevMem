package crawler

import "testing"

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		rel      string
		rules    []string
		expected bool
	}{
		{name: "exact name", base: "node_modules", rel: "node_modules", rules: []string{"node_modules"}, expected: true},
		{name: "glob lock", base: "go.sum", rel: "go.sum", rules: []string{"*.sum"}, expected: true},
		{name: "glob path", base: "bundle.min.js", rel: "web/bundle.min.js", rules: []string{"*.min.js"}, expected: true},
		{name: "no match", base: "main.go", rel: "cmd/main.go", rules: []string{"vendor", "*.sum"}, expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldIgnore(tc.base, tc.rel, tc.rules)
			if got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}
