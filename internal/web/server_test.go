package web

import "testing"

func TestBuildHTMLTemplates(t *testing.T) {
	if _, err := buildHTMLTemplates(templateFuncs); err != nil {
		t.Fatal(err)
	}
}
