package main

import (
	"fmt"
	"regexp"
	"strings"
)

var domainNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

func canonicalDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimSuffix(domain, ".")
	return strings.ToLower(domain)
}

func dashedDomain(domain string) string {
	return strings.ReplaceAll(domain, ".", "-")
}

func validateDomainName(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain cannot be empty")
	}

	if len(domain) > 253 {
		return fmt.Errorf("domain %q is too long", domain)
	}

	if strings.ContainsAny(domain, `/\`) {
		return fmt.Errorf("domain %q must not contain path separators", domain)
	}

	if !domainNamePattern.MatchString(domain) {
		return fmt.Errorf("domain %q is not a valid hostname", domain)
	}

	return nil
}
