package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	skipUp          bool
	skipReload      bool
	runtime         string
	force           bool
	allowWWW        bool
	addAliases      []string
	redirectAliases bool
)

var addCmd = &cobra.Command{
	Use:   "add [domain.com]",
	Short: "Add a new domain",
	Args:  cobra.ExactArgs(1),
	RunE:  runAddDomainCommand,
}

func runAddDomainCommand(cmd *cobra.Command, args []string) error {
	setRuntime()

	domain := canonicalDomain(args[0])
	if err := validateDomainName(domain); err != nil {
		return err
	}

	if err := confirmWWWPrimaryDomain(cmd, domain); err != nil {
		return err
	}

	db := initDB()
	defer db.Close()

	if err := setupDomain(db, domain, force); err != nil {
		return err
	}

	if len(addAliases) == 0 {
		return nil
	}

	for _, alias := range addAliases {
		if err := addDomainAlias(db, domain, alias, redirectAliases); err != nil {
			return err
		}
	}

	d, err := getDomainByName(db, domain)
	if err != nil {
		return err
	}

	if err := renderNginxForDomain(db, d); err != nil {
		return err
	}

	if !skipReload {
		if err := reloadNginx(); err != nil {
			return err
		}
	}

	return nil
}

func confirmWWWPrimaryDomain(cmd *cobra.Command, domain string) error {
	if !strings.HasPrefix(domain, "www.") {
		return nil
	}

	if allowWWW {
		return nil
	}

	apex := strings.TrimPrefix(domain, "www.")

	ok, err := promptYesNo(
		bufio.NewReader(cmd.InOrStdin()),
		fmt.Sprintf(
			"%q starts with 'www.'. Usually the primary domain should be %q, with %q as an alias. Continue anyway?",
			domain,
			apex,
			domain,
		),
		false,
	)
	if err != nil {
		return err
	}

	if !ok {
		return fmt.Errorf("aborted")
	}

	return nil
}
