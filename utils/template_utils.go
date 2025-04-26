package utils

import (
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// TemplateParse parses a template with the sprig FuncMap and additional functions.
func TemplateParse(name, tmplStr string) (*template.Template, error) {
	// add more as needed
	funcs := template.FuncMap{"truncate": truncate}
	return template.New(name).Funcs(sprig.FuncMap()).Funcs(funcs).Parse(tmplStr)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
