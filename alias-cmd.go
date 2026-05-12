package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var aliasRedirect bool

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage domain aliases",
}

var aliasAddCmd = &cobra.Command{
	Use:   "add [domain] [alias]",
	Short: "Add an alias to an existing domain",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := initDB()
		defer db.Close()

		targetDomain := canonicalDomain(args[0])
		alias := canonicalDomain(args[1])

		if err := addDomainAlias(db, targetDomain, alias, aliasRedirect); err != nil {
			return err
		}

		d, err := getDomainByName(db, targetDomain)
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

		fmt.Printf("✅ Added alias %s for %s\n", alias, targetDomain)
		return nil
	},
}

var aliasListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List aliases for a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := initDB()
		defer db.Close()

		domain := canonicalDomain(args[0])
		aliases, err := listDomainAliases(db, domain)
		if err != nil {
			return err
		}

		if len(aliases) == 0 {
			fmt.Println("No aliases found.")
			return nil
		}

		for _, alias := range aliases {
			mode := "same-site"
			if alias.Redirect {
				mode = "redirect"
			}
			fmt.Printf("- %-30s %s\n", alias.Alias, mode)
		}

		return nil
	},
}

var aliasDeleteCmd = &cobra.Command{
	Use:   "delete [alias]",
	Short: "Delete an alias",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db := initDB()
		defer db.Close()

		record, err := deleteDomainAlias(db, args[0])
		if err != nil {
			return err
		}

		d, err := getDomainByName(db, record.Domain)
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

		fmt.Printf("🗑️ Removed alias %s from %s\n", record.Alias, record.Domain)
		return nil
	},
}
