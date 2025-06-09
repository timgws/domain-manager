package main

import (
	"log"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)


var infoCmd = &cobra.Command{
	Use:   "info [domain]",
	Short: "Show detailed info for a domain",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		domain := args[0]
		db := initDB()
		defer db.Close()

		var d DomainRecord
		err := db.One("Domain", domain, &d)
		if err != nil {
			log.Fatalf("Domain not found: %s", domain)
		}

		fmt.Println("Domain    :", d.Domain)
		fmt.Println("Username  :", d.Username)
		fmt.Println("PHP Port  :", 9000+d.ID)
		fmt.Println("Created   :", d.CreatedAt.Format("2006-01-02 15:04"))
		fmt.Println("Container :", "php-"+strings.ReplaceAll(d.Domain, ".", "-"))

		var dbs []MySQLDatabaseRecord
		_ = db.Find("Domain", domain, &dbs)
		if len(dbs) > 0 {
			fmt.Println("Databases :")
			for _, db := range dbs {
				fmt.Println("  -", db.DbName)
			}
		}
	},
}
