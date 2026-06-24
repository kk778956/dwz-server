package controller

import (
	"bytes"
	"html/template"
	"strings"
	"testing"
)

func TestInstallPageDataSupportsHeaderLogoURL(t *testing.T) {
	tpl := template.Must(template.New("header").Parse(`
{{define "icon_logo"}}default-logo{{end}}
{{define "header"}}
{{if .LogoURL}}<img src="{{.LogoURL}}" alt="">{{else}}{{template "icon_logo" .}}{{end}}
{{end}}
`))

	var buf bytes.Buffer
	err := tpl.ExecuteTemplate(&buf, "header", installPageData{
		SiteName: "DWZ",
		LogoURL:  "/uploads/branding/logo.png",
	})
	if err != nil {
		t.Fatalf("execute header template: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `/uploads/branding/logo.png`) {
		t.Fatalf("expected rendered logo URL, got %q", got)
	}
}
