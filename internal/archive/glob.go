package archive

import (
	"path"
	"strings"
)

// MatchGlob matches slash-separated paths. A segment consisting of ** matches
// zero or more path segments.
func MatchGlob(pattern, name string) bool {
	pattern = strings.TrimPrefix(strings.ReplaceAll(pattern, "\\", "/"), "./")
	name = strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./")
	patterns := strings.Split(pattern, "/")
	names := strings.Split(name, "/")
	type state struct{ pattern, name int }
	memo := make(map[state]bool)
	seen := make(map[state]bool)
	var match func(int, int) bool
	match = func(patternIndex, nameIndex int) bool {
		key := state{patternIndex, nameIndex}
		if seen[key] {
			return memo[key]
		}
		seen[key] = true
		var result bool
		switch {
		case patternIndex == len(patterns):
			result = nameIndex == len(names)
		case patterns[patternIndex] == "**":
			result = match(patternIndex+1, nameIndex) ||
				(nameIndex < len(names) && match(patternIndex, nameIndex+1))
		case nameIndex < len(names):
			segmentMatches, err := path.Match(patterns[patternIndex], names[nameIndex])
			result = err == nil && segmentMatches && match(patternIndex+1, nameIndex+1)
		}
		memo[key] = result
		return result
	}
	return match(0, 0)
}
