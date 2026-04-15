package main

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all managed domains",
	Run: func(cmd *cobra.Command, args []string) {
		db := initDB()
		defer db.Close()

		var domains []DomainData
		err := db.All(&domains)
		if err != nil {
			log.Fatalf("Error fetching domains: %v", err)
		}

		if len(domains) == 0 {
			fmt.Println("No domains found.")
			return
		}

		fmt.Printf("%-4s %-30s %-20s %-20s\n", "ID", "Domain", "Username", "Created")
		for _, d := range domains {
			fmt.Printf("%-4d %-30s %-20s %-20s\n", d.ID, d.Domain, d.Username, d.CreatedAt.Format("2006-01-02 15:04"))
		}
	},
}
