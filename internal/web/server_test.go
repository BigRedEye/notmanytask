package web

import (
	"html/template"
	"path/filepath"
	"testing"
)

func TestBuildHTMLTemplates(t *testing.T) {
	_, err := buildHTMLTemplates(template.FuncMap{
		"inc": func(i int) int {
			return i + 1
		},
		"prettifyTaskName": filepath.Base,
		"add1": func(f float64) float64 {
			return f + 1.0
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
