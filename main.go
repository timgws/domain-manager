package main

import (
	"log"

	"github.com/spf13/cobra"
)

var importPath string

var rootCmd = &cobra.Command{
	Use:   "domain-manager",
	Short: "Manage Docker-based PHP-FPM domain scaffolding",
}

func main() {
	cobra.OnInitialize(initConfig)

	addCmd.Flags().BoolVar(&force, "force", false, "Force overwrite if domain already exists")
	addCmd.Flags().StringVar(&runtime, "runtime", "", "Override container runtime (podman or docker)")
	addCmd.Flags().BoolVar(&skipUp, "no-up", false, "Skip container startup")
	addCmd.Flags().BoolVar(&skipReload, "no-reload", false, "Skip nginx reload")

	addCmd.Flags().BoolVar(&allowWWW, "allow-www", false, "Allow adding a primary domain starting with www. without prompting")
	addCmd.Flags().StringArrayVar(&addAliases, "alias", nil, "Alias domain for this site")
	addCmd.Flags().BoolVar(&redirectAliases, "redirect-aliases", false, "Set aliases up as redirects to the primary domain")

	bootstrapCmd.Flags().BoolVar(&forceBootstrap, "force", false, "overwrite contents if target directory is not empty")
	enableSSL.Flags().Bool("cloudflare", false, "Force Cloudflare DNS validation")
	enableSSL.Flags().Bool("no-reload", false, "Skip nginx reload after issuing certificates")
	backupCmd.Flags().StringVar(&backupOutputDir, "output-dir", "", "Directory to write the backup archive to")

	mysqlCreateCmd.Flags().StringVar(&importPath, "import", "", "Optional path to SQL file to import")

	mysqlListCmd.Flags().Bool("stats", false, "Show size statistics for databases")
	mysqlCmd.AddCommand(mysqlCreateCmd)
	mysqlCmd.AddCommand(mysqlListCmd)
	mysqlCmd.AddCommand(mysqlDeleteCmd)

	migrateDomainsCmd.Flags().BoolVar(&migrateDomainsDryRun, "dry-run", false, "Show what would be migrated without writing anything")
	migrateDomainsCmd.Flags().BoolVar(&migrateDomainsForce, "force", false, "Replace existing DomainData records before migrating")
	migrateDomainsCmd.Flags().BoolVar(&migrateDomainsDropOld, "drop-old-bucket", false, "Drop the old DomainRecord bucket after a successful migration")

	aliasAddCmd.Flags().BoolVar(&aliasRedirect, "redirect", false, "Redirect this alias to the canonical domain")
	aliasCmd.AddCommand(aliasAddCmd)
	aliasCmd.AddCommand(aliasListCmd)
	aliasCmd.AddCommand(aliasDeleteCmd)
	rootCmd.AddCommand(aliasCmd)

	rootCmd.AddCommand(migrateDomainsCmd)

	rootCmd.AddCommand(mysqlCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(enableSSL)
	rootCmd.AddCommand(installDepsCmd)
	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(backupCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

