package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/asdine/storm/v3"
)

type DomainAliasRecord struct {
	ID           int       `storm:"id,increment"`
	Domain       string    `storm:"index"`  // canonical/target domain
	Alias        string    `storm:"unique"` // alias domain
	Redirect     bool
	CreatedAt    time.Time
	DomainDashed string
}

func addDomainAlias(db *storm.DB, targetDomain, alias string, redirect bool) error {
	targetDomain = canonicalDomain(targetDomain)
	alias = canonicalDomain(alias)

	if err := validateDomainName(targetDomain); err != nil {
		return fmt.Errorf("invalid target domain %q: %w", targetDomain, err)
	}
	if err := validateDomainName(alias); err != nil {
		return fmt.Errorf("invalid alias %q: %w", alias, err)
	}
	if alias == targetDomain {
		return fmt.Errorf("alias %q is the same as the target domain", alias)
	}

	var target DomainData
	if err := db.One("Domain", targetDomain, &target); err != nil {
		return fmt.Errorf("target domain %q is not managed: %w", targetDomain, err)
	}

	var existingDomain DomainData
	if err := db.One("Domain", alias, &existingDomain); err == nil {
		return fmt.Errorf("alias %q is already managed as a primary domain", alias)
	} else if !errors.Is(err, storm.ErrNotFound) {
		return fmt.Errorf("failed checking whether alias is a primary domain: %w", err)
	}

	var existingAlias DomainAliasRecord
	if err := db.One("Alias", alias, &existingAlias); err == nil {
		return fmt.Errorf("alias %q already points to %q", alias, existingAlias.Domain)
	} else if !errors.Is(err, storm.ErrNotFound) {
		return fmt.Errorf("failed checking existing aliases: %w", err)
	}

	record := DomainAliasRecord{
		Domain:       targetDomain,
		Alias:        alias,
		Redirect:     redirect,
		CreatedAt:    time.Now(),
		DomainDashed: dashedDomain(alias),
	}

	if err := db.Save(&record); err != nil {
		return fmt.Errorf("failed to save alias %q: %w", alias, err)
	}

	return nil
}

func listDomainAliases(db *storm.DB, domain string) ([]DomainAliasRecord, error) {
	domain = canonicalDomain(domain)

	var aliases []DomainAliasRecord
	if err := db.Find("Domain", domain, &aliases); err != nil {
		if errors.Is(err, storm.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list aliases for %q: %w", domain, err)
	}

	return aliases, nil
}

func deleteDomainAlias(db *storm.DB, alias string) (*DomainAliasRecord, error) {
	alias = canonicalDomain(alias)

	var record DomainAliasRecord
	if err := db.One("Alias", alias, &record); err != nil {
		return nil, fmt.Errorf("alias %q not found: %w", alias, err)
	}

	if err := db.DeleteStruct(&record); err != nil {
		return nil, fmt.Errorf("failed to delete alias %q: %w", alias, err)
	}

	return &record, nil
}
