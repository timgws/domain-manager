package main

import (
	"path/filepath"
	"strings"
	"text/template"
)

func renderTemplateWithFuncs(tmplPath string, data any) (string, error) {
	tmpl, err := template.New(filepath.Base(tmplPath)).Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf strings.Builder

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderTemplate(tmplPath string, data any) (string, error) {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf strings.Builder

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
