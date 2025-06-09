package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
	"math/rand"
	"strconv"
	"strings"
	"os"
	"os/exec"

	"github.com/martinhoefling/goxkcdpwgen/xkcdpwgen"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	_ "github.com/go-sql-driver/mysql"
)

type MySQLDatabaseRecord struct {
	ID        int       `storm:"id,increment"`
	Domain    string    `storm:"index"`
	DbName    string    `storm:"unique"`
	CreatedAt time.Time
}

func generatePassword() string {
	gen := xkcdpwgen.NewGenerator()
	gen.SetNumWords(4)
	gen.SetCapitalize(true)
	gen.SetDelimiter("@")

	rand.Seed(time.Now().UnixNano())
	num := rand.Intn(90) + 10
	numStr := strconv.Itoa(num)

	return gen.GeneratePasswordString() + numStr
}

var mysqlCmd = &cobra.Command{
	Use:   "mysql",
	Short: "Manage MySQL databases",
}

var mysqlCreateCmd = &cobra.Command{
	Use:   "create [domain.com] [dbname]",
	Short: "Create a new MySQL database for a domain",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		domain := args[0]
		dbName := args[1]

		if importPath != "" {
			if !strings.HasSuffix(importPath, ".sql") {
				log.Fatalf("import path must be a .sql file")
			}

			info, err := os.Stat(importPath)
			if os.IsNotExist(err) {
				log.Fatalf("import file does not exist: %s", importPath)
			}
			if err != nil {
				log.Fatalf("unable to access import file: %w", err)
			}
			if info.IsDir() {
				log.Fatalf("import path is a directory, not a file: %s", importPath)
			}
		}

		db := initDB()
		defer db.Close()

		mysqlPassword := generatePassword()

		mysql, err := connectToMySQL()
		if err != nil {
			log.Fatalf("MySQL connection failed: %v", err)
		}
		defer mysql.Close()

		username := fmt.Sprintf("db_%s", dbName)
		if len(username) > 32 {
			username = username[:32]
		}

		queries := []string{
			fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName),
			fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", username, mysqlPassword),
			fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'", dbName, username),
			"FLUSH PRIVILEGES",
		}
		for _, q := range queries {
			if _, err := mysql.Exec(q); err != nil {
				log.Fatalf("Error running query: %v", err)
			}
		}

		err = importSqlFile(importPath, username, dbName, mysqlPassword)
		if err != nil {
			log.Printf("Could not import SQL file: %v", err)
		}

		// Save to storm (but not password)
		record := MySQLDatabaseRecord{
			Domain:    domain,
			DbName:    dbName,
			CreatedAt: time.Now(),
		}
		if err := db.Save(&record); err != nil {
			log.Fatalf("Failed to save DB record: %v", err)
		}

		fmt.Println("✅ MySQL database created successfully!")
		fmt.Println("Database name:", dbName)
		fmt.Println("Username     :", username)
		fmt.Println("Password     :", mysqlPassword)
	},
}

var mysqlListCmd = &cobra.Command{
	Use:   "list [domain.com]",
	Short: "List all MySQL databases for a domain",
	Args:  cobra.ExactArgs(1),
	Run:   mysqlListHandler,
}

var mysqlDeleteCmd = &cobra.Command{
	Use:   "delete [domain.com] [dbname]",
	Short: "Delete a MySQL database",
	Args:  cobra.ExactArgs(2),
	Run:   mysqlDeleteHandler,
}

func connectToMySQL() (*sql.DB, error) {
	mysqlRootUser := viper.GetString("mysql.root_user")
	mysqlRootPass := viper.GetString("mysql.root_password")
	mysqlHost := viper.GetString("mysql.host")

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/", mysqlRootUser, mysqlRootPass, mysqlHost)
	return sql.Open("mysql", dsn)
}

func mysqlListHandler(cmd *cobra.Command, args []string) {
	showStats, _ := cmd.Flags().GetBool("stats")
	db := initDB()
	defer db.Close()

	var records []MySQLDatabaseRecord

	if len(args) == 1 && args[0] != "" {
		domain := args[0]
		if err := db.Find("Domain", domain, &records); err != nil {
			log.Fatalf("Failed to list databases: %v", err)
		}
	} else {
		if err := db.All(&records); err != nil {
			log.Fatalf("Failed to list databases: %v", err)
		}
	}

	if len(records) == 0 {
		fmt.Println("No databases found.")
		return
	}

	fmt.Println("📦 MySQL Databases:")

	// Optional stats
	var stats map[string]float64
	if showStats {
		var err error
		stats, err = fetchDatabaseStats()
		if err != nil {
			log.Printf("⚠️  Failed to get stats: %v", err)
		}
	}

	for _, r := range records {
		if showStats {
			size := stats[r.DbName]
			fmt.Printf("- %-30s (domain: %-20s)  %6.2f MB\n", r.DbName, r.Domain, size)
		} else {
			fmt.Printf("- %-30s (domain: %-20s)  created %s\n", r.DbName, r.Domain, r.CreatedAt.Format(time.RFC822))
		}
	}
}

func mysqlDeleteHandler(cmd *cobra.Command, args []string) {
	domain := args[0]
	dbName := args[1]

	db := initDB()
	defer db.Close()

	var record MySQLDatabaseRecord
	if err := db.One("DbName", dbName, &record); err != nil {
		log.Fatalf("Database record not found: %v", err)
	}
	if record.Domain != domain {
		log.Fatalf("Database %s does not belong to domain %s", dbName, domain)
	}

	mysql, err := connectToMySQL()
	if err != nil {
		log.Fatalf("MySQL connection failed: %v", err)
	}
	defer mysql.Close()

	username := fmt.Sprintf("db_%s", dbName)
	if len(username) > 32 {
		username = username[:32]
	}

	queries := []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName),
		fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%'", username),
		"FLUSH PRIVILEGES",
	}
	for _, q := range queries {
		if _, err := mysql.Exec(q); err != nil {
			log.Fatalf("Error running query: %v", err)
		}
	}

	if err := db.DeleteStruct(&record); err != nil {
		log.Fatalf("Failed to delete database record: %v", err)
	}

	fmt.Printf("🗑️  MySQL database %s and user %s deleted successfully\n", dbName, username)
}

func importSqlFile(importPath string, username string, dbName string, password string) error {
	if importPath == "" {
		return nil;
	}

	cmd := exec.Command("mysql",
		"-u", username,
		"-p" + password,
		"-h", "127.0.0.1",
		"-P", "3306",
		dbName,
	)

	sqlFile, _ := os.Open(importPath)
	defer sqlFile.Close()
	cmd.Stdin = sqlFile
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("📥 Importing SQL into %s...\n", dbName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mysql import failed: %w", err)
	}

	return nil
}

func fetchDatabaseStats() (map[string]float64, error) {
	mysql, err := connectToMySQL()
	if err != nil {
		return nil, err
	}
	defer mysql.Close()

	rows, err := mysql.Query(`
		SELECT table_schema AS db, 
		       ROUND(SUM(data_length + index_length) / 1024 / 1024, 2) AS size_mb
		FROM information_schema.tables
		GROUP BY table_schema
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var dbName string
		var sizeMB float64
		if err := rows.Scan(&dbName, &sizeMB); err != nil {
			continue
		}
		result[dbName] = sizeMB
	}
	return result, nil
}
