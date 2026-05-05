package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [domain]",
	Short: "Delete a domain and its config, data, and runtime resources",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		setRuntime()
		db := initDB()
		defer db.Close()

		domain, err := getDomainByName(db, args[0])
		if err != nil {
			return err
		}

		reader := bufio.NewReader(os.Stdin)

		confirmCoreDelete, err := promptYesNo(
			reader,
			fmt.Sprintf("Delete domain '%s' container, nginx config, and domain record?", domain.Domain),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmCoreDelete {
			fmt.Println("Aborted.")
			return nil
		}

		deleteFiles, err := promptYesNo(
			reader,
			fmt.Sprintf("Delete website files at %s?", filepath.Join(websitesRoot, domain.Domain)),
			false,
		)
		if err != nil {
			return err
		}

		deleteMySQL, err := promptYesNo(
			reader,
			fmt.Sprintf("Delete MySQL databases and users for '%s'?", domain.Domain),
			false,
		)
		if err != nil {
			return err
		}

		deleteSSL, err := promptYesNo(
			reader,
			fmt.Sprintf("Delete Let's Encrypt files for '%s'?", domain.Domain),
			false,
		)
		if err != nil {
			return err
		}

		deleteUser, err := promptYesNo(
			reader,
			fmt.Sprintf("Delete Unix user '%s' and jail link?", domain.Username),
			false,
		)
		if err != nil {
			return err
		}

		var cleanupErrs []error
		composeDir := filepath.Join(viper.GetString("base_output_dir"), domain.Domain)
		if err := runComposeDown(composeDir, runtime); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}

		nginxConf := filepath.Join(nginxConfDir(), domain.DomainDashed+".conf")
		if err := removePathIfExists(nginxConf); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}

		if deleteMySQL {
			if err := removeDomainMySQLResources(db, domain.Domain); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
		}

		if deleteFiles {
			if err := removePathIfExists(filepath.Join(websitesRoot, domain.Domain)); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
		}

		if deleteSSL {
			if err := removeDomainCertificates(domain.Domain); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
		}

		if deleteUser {
			if err := removePathIfExists(filepath.Join(jailhomesRoot, domain.Username)); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
			if userExists(domain.Username) {
				cmd := exec.Command("userdel", "-r", domain.Username)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to delete user %s: %w", domain.Username, err))
				}
			}
		}
		if err := db.DeleteStruct(domain); err != nil && !errors.Is(err, storm.ErrNotFound) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to remove domain record: %w", err))
		}

		if err := reloadNginx(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to reload nginx: %w", err))
		}

		if len(cleanupErrs) > 0 {
			return errors.Join(cleanupErrs...)
		}

		fmt.Println("✅ Domain deleted.")
		return nil
	},
}

func removeDomainMySQLResources(db *storm.DB, domain string) error {
	var records []MySQLDatabaseRecord
	if err := db.Find("Domain", domain, &records); err != nil && !errors.Is(err, storm.ErrNotFound) {
		return fmt.Errorf("failed to look up MySQL databases for %s: %w", domain, err)
	}
	if len(records) == 0 {
		return nil
	}

	mysqlDB, err := connectToMySQL()
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL for cleanup: %w", err)
	}
	defer mysqlDB.Close()

	var errs []error
	for _, record := range records {
		username := fmt.Sprintf("db_%s", record.DbName)
		if len(username) > 32 {
			username = username[:32]
		}

		queries := []string{
			fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", record.DbName),
			fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", username),
		}
		for _, q := range queries {
			if _, err := mysqlDB.Exec(q); err != nil {
				errs = append(errs, fmt.Errorf("failed to clean up MySQL resource for %s: %w", record.DbName, err))
			}
		}
		if err := db.DeleteStruct(&record); err != nil && !errors.Is(err, storm.ErrNotFound) {
			errs = append(errs, fmt.Errorf("failed to remove MySQL record for %s: %w", record.DbName, err))
		}
	}

	if _, err := mysqlDB.Exec("FLUSH PRIVILEGES"); err != nil {
		errs = append(errs, fmt.Errorf("failed to flush MySQL privileges: %w", err))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func removeDomainCertificates(domain string) error {
	var errs []error
	paths := []string{
		filepath.Join(viper.GetString("letsencrypt_live_dir"), domain),
		filepath.Join(viper.GetString("letsencrypt_archive_dir"), domain),
		filepath.Join(viper.GetString("letsencrypt_renewal_dir"), domain+".conf"),
	}

	for _, path := range paths {
		if err := removePathIfExists(path); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func removePathIfExists(path string) error {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to remove %s: %w", path, err)
	}
	return nil
}
