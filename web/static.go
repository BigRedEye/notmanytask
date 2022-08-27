package web

import "embed"

//go:embed *.html *.css
var StaticContent embed.FS

//go:embed *.tmpl
var StaticTemplates embed.FS
