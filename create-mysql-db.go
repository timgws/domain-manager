package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
	"math/rand"
	"strconv"

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

var createDBCmd = &cobra.Command{
	Use:   "create-mysql-db [domain.com] [dbname]",
	Short: "Create a new MySQL database for a domain",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		domain := args[0]
		dbName := args[1]

		db := initDB()
		defer db.Close()


		mysqlPassword := generatePassword()
		log.Print(mysqlPassword)

		mysqlRootUser := viper.GetString("mysql.root_user")
		mysqlRootPass := viper.GetString("mysql.root_password")
		mysqlHost := viper.GetString("mysql.host")

		dsn := fmt.Sprintf("%s:%s@tcp(%s)/", mysqlRootUser, mysqlRootPass, mysqlHost)
		mysql, err := sql.Open("mysql", dsn)
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
