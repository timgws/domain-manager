package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var backupOutputDir string

var backupCmd = &cobra.Command{
	Use:   "backup [domain]",
	Short: "Create a tar.bz2 backup of a domain's website root and MySQL databases",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := exec.LookPath("tar"); err != nil {
			return fmt.Errorf("tar not found in PATH: %w", err)
		}

		domain, err := GetByName(args[0])
		if err != nil {
			return err
		}

		siteRoot := filepath.Join(websitesRoot, domain.Domain)
		if _, err := os.Stat(siteRoot); err != nil {
			return fmt.Errorf("website root not accessible at %s: %w", siteRoot, err)
		}

		db := initDB()
		defer db.Close()

		var dbRecords []MySQLDatabaseRecord
		if err := db.Find("Domain", domain.Domain, &dbRecords); err != nil && !errors.Is(err, storm.ErrNotFound) {
			return fmt.Errorf("failed to look up databases for %s: %w", domain.Domain, err)
		}

		outputDir := backupOutputDir
		if outputDir == "" {
			outputDir = viper.GetString("backup_output_dir")
		}
		if outputDir == "" {
			outputDir = "."
		}
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return err
		}

		timestamp := time.Now().Format("20060102-150405")
		backupBase := fmt.Sprintf("%s-%s", domain.DomainDashed, timestamp)
		archivePath := filepath.Join(outputDir, backupBase+".tar.bz2")

		stageDir, err := os.MkdirTemp("", "domain-backup-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stageDir)

		backupRoot := filepath.Join(stageDir, backupBase)
		if err := os.MkdirAll(backupRoot, 0o755); err != nil {
			return err
		}

		if err := writeBackupMetadata(backupRoot, domain, dbRecords); err != nil {
			return err
		}
		if err := os.Symlink(siteRoot, filepath.Join(backupRoot, "site")); err != nil {
			return fmt.Errorf("failed to stage website root: %w", err)
		}
		if err := dumpDatabasesForBackup(backupRoot, dbRecords); err != nil {
			return err
		}

		archiveCmd := exec.Command("tar", "-chjf", archivePath, "-C", stageDir, backupBase)
		archiveCmd.Stdout = os.Stdout
		archiveCmd.Stderr = os.Stderr
		if err := archiveCmd.Run(); err != nil {
			return fmt.Errorf("failed to create archive %s: %w", archivePath, err)
		}

		fmt.Println("✅ Backup written to", archivePath)
		return nil
	},
}

func writeBackupMetadata(backupRoot string, domain *DomainData, dbRecords []MySQLDatabaseRecord) error {
	var builder strings.Builder
	builder.WriteString("domain: " + domain.Domain + "\n")
	builder.WriteString("username: " + domain.Username + "\n")
	builder.WriteString("created_at: " + domain.CreatedAt.Format(time.RFC3339) + "\n")
	builder.WriteString("backup_created_at: " + time.Now().Format(time.RFC3339) + "\n")
	builder.WriteString("website_root: " + filepath.Join(websitesRoot, domain.Domain) + "\n")
	builder.WriteString("databases:\n")
	for _, record := range dbRecords {
		builder.WriteString("  - " + record.DbName + "\n")
	}

	return os.WriteFile(filepath.Join(backupRoot, "metadata.txt"), []byte(builder.String()), 0o644)
}

func dumpDatabasesForBackup(backupRoot string, dbRecords []MySQLDatabaseRecord) error {
	if len(dbRecords) == 0 {
		return nil
	}

	if _, err := exec.LookPath("mysqldump"); err != nil {
		return fmt.Errorf("mysqldump not found in PATH: %w", err)
	}

	dumpDir := filepath.Join(backupRoot, "databases")
	if err := os.MkdirAll(dumpDir, 0o755); err != nil {
		return err
	}

	host, port := mysqlHostPortFromConfig()
	rootUser := viper.GetString("mysql.root_user")
	rootPass := viper.GetString("mysql.root_password")
	if rootUser == "" {
		return fmt.Errorf("mysql.root_user must be configured for backups")
	}

	for _, record := range dbRecords {
		dumpPath := filepath.Join(dumpDir, record.DbName+".sql")
		if err := dumpSingleDatabase(record.DbName, dumpPath, rootUser, rootPass, host, port); err != nil {
			return err
		}
	}

	return nil
}

func dumpSingleDatabase(dbName, dumpPath, user, password, host, port string) error {
	file, err := os.Create(dumpPath)
	if err != nil {
		return fmt.Errorf("failed to create dump file for %s: %w", dbName, err)
	}
	defer file.Close()

	cmd := exec.Command(
		"mysqldump",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
		"--protocol=tcp",
		"-u", user,
		"-h", host,
		"-P", port,
		dbName,
	)
	cmd.Env = append(os.Environ(), "MYSQL_PWD="+password)
	cmd.Stdout = file
	cmd.Stderr = os.Stderr

	fmt.Printf("🗄️  Dumping database %s...\n", dbName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysqldump failed for %s: %w", dbName, err)
	}
	return nil
}
