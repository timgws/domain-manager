package main

import (
	"log"
	"fmt"
	"strings"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [domain]",
	Short: "Delete a domain and optionally its config/container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		domain := args[0]
		db := initDB()
		defer db.Close()

		var record DomainRecord
		err := db.One("Domain", domain, &record)
		if err != nil {
			log.Fatalf("Domain not found: %s", domain)
		}

		fmt.Printf("Are you sure you want to delete domain '%s'? [y/N]: ", domain)
		var input string
		fmt.Scanln(&input)
		if strings.ToLower(input) != "y" {
			fmt.Println("Aborted.")
			return
		}

		// Remove Compose container
		dir := filepath.Join(viper.GetString("base_output_dir"), domain)
		cmdDown := exec.Command("podman-compose", "down")
		cmdDown.Dir = dir
		_ = cmdDown.Run()

		// Remove nginx config
		conf := filepath.Join("/etc/nginx/conf.d", strings.ReplaceAll(domain, ".", "-")+".conf")
		_ = os.Remove(conf)

		// Remove from DB
		if err := db.DeleteStruct(&record); err != nil {
			log.Fatalf("Failed to remove domain record: %v", err)
		}

		// Optionally remove site folder
		// _ = os.RemoveAll(dir)

		fmt.Println("✅ Domain deleted.")
	},
}

