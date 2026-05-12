package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/spf13/viper"
)

func setupDomain(db *storm.DB, domain string, force bool) error {
	existing, err := createOrLoadDomainRecord(db, domain, force)
	if err != nil {
		return err
	}

	if err := ensureDomainUnixUser(existing); err != nil {
		return err
	}

	enrichDomain(existing)

	if existing.UID == 0 || existing.GID == 0 {
		return fmt.Errorf("failed to determine UID/GID for %s", existing.Username)
	}

	outputDir := domainOutputDir(existing)

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	if err := writeDomainTemplates(existing, outputDir); err != nil {
		return err
	}

	if err := ensureDomainFilesystem(existing); err != nil {
		return err
	}

	if err := renderNginxForDomain(db, existing); err != nil {
		return err
	}

	if err := maybeStartDomainRuntime(outputDir, force); err != nil {
		return err
	}

	if err := maybeReloadNginx(); err != nil {
		return err
	}

	return nil
}

func createOrLoadDomainRecord(db *storm.DB, domain string, force bool) (*DomainData, error) {
	existing, err := getDomainByName(db, domain)

	switch {
	case err == nil && !force:
		return nil, fmt.Errorf("domain %q already exists", domain)

	case err == nil && force:
		fmt.Println("⚠️ Domain exists, but proceeding due to --force.")
		return existing, nil

	case errors.Is(err, storm.ErrNotFound):
		record := &DomainData{
			Domain:       domain,
			Username:     generateUsername(domain),
			CreatedAt:    time.Now(),
			DomainDashed: dashedDomain(domain),
			PHPVersion:   viper.GetString("default_php_version"),
		}

		if err := db.Save(record); err != nil {
			return nil, fmt.Errorf("failed to save domain record: %w", err)
		}

		return record, nil

	default:
		return nil, err
	}
}

func ensureDomainUnixUser(domain *DomainData) error {
	if domain.Username == "" {
		domain.Username = generateUsername(domain.Domain)
	}

	if userExists(domain.Username) {
		return nil
	}

	fmt.Println("Creating user:", domain.Username)

	if err := createUnixUser(domain.Username); err != nil {
		return fmt.Errorf("could not create user: %w", err)
	}

	return nil
}

func writeDomainTemplates(domain *DomainData, outputDir string) error {
	templates := []string{
		"docker-compose.yml.tmpl",
		"php-fpm.Dockerfile.tmpl",
	}

	for _, tmpl := range templates {
		if err := writeDomainTemplate(domain, outputDir, tmpl); err != nil {
			return err
		}
	}

	return nil
}

func writeDomainTemplate(domain *DomainData, outputDir, templateName string) error {
	tmplPath := filepath.Join(viper.GetString("template_dir"), templateName)

	rendered, err := renderTemplateWithFuncs(tmplPath, *domain)
	if err != nil {
		return fmt.Errorf("failed to render template %s: %w", templateName, err)
	}

	dest := filepath.Join(outputDir, strings.TrimSuffix(templateName, ".tmpl"))

	if err := os.WriteFile(dest, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("failed to write template output %s: %w", dest, err)
	}

	fmt.Println("Generated:", dest)

	return nil
}

func ensureDomainFilesystem(domain *DomainData) error {
	domainPath := domainWebsitePath(domain)
	jailLink := domainJailLink(domain)

	if err := os.MkdirAll(domainPath, 0o755); err != nil {
		return fmt.Errorf("failed to create domain dir: %w", err)
	}

	if err := os.MkdirAll(jailhomesRoot, 0o755); err != nil {
		return fmt.Errorf("failed to create jailhomes root dir: %w", err)
	}

	if _, err := os.Lstat(jailLink); os.IsNotExist(err) {
		if err := os.Symlink(domainPath, jailLink); err != nil {
			return fmt.Errorf("failed to create jail symlink: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to inspect jail symlink %s: %w", jailLink, err)
	}

	return nil
}

func maybeStartDomainRuntime(outputDir string, force bool) error {
	if skipUp {
		return nil
	}

	if err := runComposeUp(outputDir, runtime, force); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

func maybeReloadNginx() error {
	if skipReload {
		return nil
	}

	if err := reloadNginx(); err != nil {
		return fmt.Errorf("failed to reload nginx: %w", err)
	}

	return nil
}

func domainOutputDir(domain *DomainData) string {
	return filepath.Join(viper.GetString("base_output_dir"), domain.Domain)
}

func domainWebsitePath(domain *DomainData) string {
	return filepath.Join(websitesRoot, domain.Domain)
}

func domainJailLink(domain *DomainData) string {
	return filepath.Join(jailhomesRoot, domain.Username)
}
