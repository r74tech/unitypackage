package archive

import (
	"strings"
	"testing"
)

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"Assets/**/*.cs", "Assets/Test.cs", true},
		{"Assets/**/*.cs", "Assets/Nested/Test.cs", true},
		{"Assets/*.cs", "Assets/Nested/Test.cs", false},
		{"**/*.png", "Packages/com.example/Logo.png", true},
		{"Assets/?est.cs", "Assets/Test.cs", true},
	}
	for _, testCase := range cases {
		if got := MatchGlob(testCase.pattern, testCase.name); got != testCase.want {
			t.Errorf("MatchGlob(%q, %q) = %v, want %v", testCase.pattern, testCase.name, got, testCase.want)
		}
	}
}

func TestMatchGlobHandlesManyDoubleStarsWithoutExponentialBacktracking(t *testing.T) {
	pattern := strings.Repeat("**/", 100) + "target"
	name := strings.Repeat("segment/", 100) + "target"
	if !MatchGlob(pattern, name) {
		t.Fatal("expected pathological double-star pattern to match")
	}
}
