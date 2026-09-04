package render

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/pkg/errors"
)

// Patterns are gitignore-like globs matched against a whole slash-separated
// path relative to the source root: `*` and `?` stay within one path
// segment, `**` spans any number of segments.
type glob string

func compileGlobs(patterns []string) ([]glob, error) {
	globs := make([]glob, 0, len(patterns))
	for _, pattern := range patterns {
		if !doublestar.ValidatePattern(pattern) {
			return nil, errors.Errorf("bad pattern %q", pattern)
		}
		globs = append(globs, glob(pattern))
	}
	return globs, nil
}

func anyMatch(globs []glob, path string) bool {
	for _, g := range globs {
		if ok, _ := doublestar.Match(string(g), path); ok {
			return true
		}
	}
	return false
}
