package render

import "testing"

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false},
		{"**/*.md", "README.md", true},
		{"**/*.md", "docs/a/b/README.md", true},
		{"cmake/**", "cmake/x.cmake", true},
		{"cmake/**", "cmake/a/b.cmake", true},
		{"cmake/**", "cmakes/x", false},
		{"**/private/**", "tasks/x/private/t.cpp", true},
		{"**/private/**", "private/t.cpp", true},
		{"**/private/**", "tasks/x/privates/t.cpp", false},
		{".gitlab-ci.yml", ".gitlab-ci.yml", true},
		{"tasks/?/x", "tasks/a/x", true},
		{"tasks/?/x", "tasks/ab/x", false},
	}
	for _, c := range cases {
		globs, err := compileGlobs([]string{c.pattern})
		if err != nil {
			t.Fatalf("%q: %v", c.pattern, err)
		}
		if got := anyMatch(globs, c.path); got != c.want {
			t.Errorf("%q vs %q: got %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}
