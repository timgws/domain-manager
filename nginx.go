package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/asdine/storm/v3"
	"github.com/spf13/viper"
)

type NginxDomainTemplateData struct {
	DomainData
	ServerNames      []string
	SiteAliases      []DomainAliasRecord
	RedirectAliases  []DomainAliasRecord
}

func renderNginxForDomain(db *storm.DB, domain *DomainData) error {
	aliases, err := listDomainAliases(db, domain.Domain)
	if err != nil {
		return err
	}

	data := NginxDomainTemplateData{
		DomainData:  *domain,
		ServerNames: []string{domain.Domain},
	}

	for _, alias := range aliases {
		if alias.Redirect {
			data.RedirectAliases = append(data.RedirectAliases, alias)
			continue
		}

		data.SiteAliases = append(data.SiteAliases, alias)
		data.ServerNames = append(data.ServerNames, alias.Alias)
	}

	nginxPath := filepath.Join(nginxConfDir(), domain.DomainDashed+".conf")
	tmplPath := filepath.Join(viper.GetString("template_dir"), "nginx.conf.tmpl")

	nginxConf, err := renderTemplateWithFuncs(tmplPath, data)
	if err != nil {
		return fmt.Errorf("failed to render nginx config: %w", err)
	}

	if err := os.WriteFile(nginxPath, []byte(nginxConf), 0o644); err != nil {
		return fmt.Errorf("failed to write nginx config: %w", err)
	}

	fmt.Println("✅ Nginx config written to", nginxPath)
	return nil
}
