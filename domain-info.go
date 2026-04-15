package main

import (
	"fmt"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info [domain]",
	Short: "Show detailed info for a domain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := GetByName(args[0])
		if err != nil {
			return err
		}

		fmt.Println("Domain    :", d.Domain)
		fmt.Println("Username  :", d.Username)
		fmt.Println("PHP Port  :", 9000+d.ID)
		fmt.Println("Created   :", d.CreatedAt.Format("2006-01-02 15:04"))
		fmt.Println("Container :", "php-"+d.DomainDashed)

		db := initDB()
		defer db.Close()

		var dbs []MySQLDatabaseRecord
		if err := db.Find("Domain", d.Domain, &dbs); err != nil && err != storm.ErrNotFound {
			return err
		}
		if len(dbs) > 0 {
			fmt.Println("Databases :")
			for _, dbRecord := range dbs {
				fmt.Println("  -", dbRecord.DbName)
			}
		}
		return nil
	},
}
