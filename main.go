package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type DomainRecord struct {
	ID        int       `storm:"id,increment"`
	Domain    string    `storm:"unique"`
	Username  string
	CreatedAt time.Time
}

type DomainData struct {
	ID           int
	Domain       string
	DomainDashed string
	PHPVersion   string
	Username     string
}

var websitesRoot string
var jailhomesRoot string

var skipUp bool
var skipReload bool
var runtime string
var importPath string
var force bool

var rootCmd = &cobra.Command{
	Use:   "domain-manager",
	Short: "Manage Docker-based PHP-FPM domain scaffolding",
}

var addCmd = &cobra.Command{
	Use:   "add [domain.com]",
	Short: "Add a new domain",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		setRuntime()
		domain := args[0]
		db := initDB()
		defer db.Close()
		if err := setupDomain(db, domain, force); err != nil {
			log.Fatal("Error:", err)
		}
	},
}

func main() {
	cobra.OnInitialize(initConfig)

	addCmd.Flags().BoolVar(&force, "force", false, "Force overwrite if domain already exists")
	addCmd.Flags().StringVar(&runtime, "runtime", "", "Override container runtime (podman or docker)")
	addCmd.Flags().BoolVar(&skipUp, "no-up", false, "Skip container startup")
	addCmd.Flags().BoolVar(&skipReload, "no-reload", false, "Skip nginx reload")

	bootstrapCmd.Flags().BoolVar(&forceBootstrap, "force", false, "overwrite contents if target directory is not empty")

	mysqlCreateCmd.Flags().StringVar(&importPath, "import", "", "Optional path to SQL file to import")

	mysqlListCmd.Flags().Bool("stats", false, "Show size statistics for databases")
	mysqlCmd.AddCommand(mysqlCreateCmd)
	mysqlCmd.AddCommand(mysqlListCmd)
	mysqlCmd.AddCommand(mysqlDeleteCmd)
	rootCmd.AddCommand(mysqlCmd)

	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(enableSSL)
	rootCmd.AddCommand(installDepsCmd)
	rootCmd.AddCommand(bootstrapCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/domain-manager/")
	viper.AddConfigPath(".") // allow local override
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config: %v", err)
	}

	websitesRoot = viper.GetString("websites_root")
	jailhomesRoot = viper.GetString("jailhomes_root")

	if websitesRoot == "" || jailhomesRoot == "" {
		log.Fatal("paths.websites_root and paths.jailhomes_root must be configured")
	}
}

func initDB() *storm.DB {
	dbPath := viper.GetString("database_path")
	db, err := storm.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open Storm DB: %v", err)
	}
	return db
}


func GetByName(name string) (*DomainData, error) {
	db := initDB()
	var d DomainData
	err := db.One("Name", name, &d)
	if err != nil {
		return nil, fmt.Errorf("domain %s not found: %w", name, err)
	}
	return &d, nil
}

func setRuntime() {
	if runtime == "" {
		runtime = viper.GetString("default_runtime")
		if runtime == "" {
			runtime = "podman"
		}
	}
	if runtime != "podman" && runtime != "docker" {
		log.Fatalf("Invalid runtime: %s (must be 'podman' or 'docker')", runtime)
	}
}

func runComposeUp(domainDir, runtime string, force bool) error {
	var downCmd *exec.Cmd
	var upCmd *exec.Cmd

	if runtime == "docker" {
		downCmd = exec.Command("docker-compose", "down")
		upCmd = exec.Command("docker-compose", "up", "--build", "-d")
	} else {
		downCmd = exec.Command("podman-compose", "down")
		upCmd = exec.Command("podman-compose", "up", "--build", "-d")
	}

	downCmd.Dir = domainDir
	upCmd.Dir = domainDir
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr

	if force {
		fmt.Printf("🔁 Force mode: bringing down existing %s-compose stack...\n", runtime)
		if err := downCmd.Run(); err != nil {
			log.Printf("⚠️ Failed to run %s-compose down: %v", runtime, err)
		}
	}

	fmt.Printf("🌀 Starting %s-compose for %s...\n", runtime, domainDir)
	return upCmd.Run()
}

func generateUsername(domain string) string {
	base := "wp_" + strings.ReplaceAll(domain, ".", "_")
	if len(base) > 30 {
		base = base[:30]
	}
	return base
}

func userExists(username string) bool {
	cmd := exec.Command("id", "-u", username)
	return cmd.Run() == nil
}

func createUnixUser(username string) error {
	// Ensure the sftpusers group exists
	if err := exec.Command("getent", "group", "sftpusers").Run(); err != nil {
		if err := exec.Command("groupadd", "sftpusers").Run(); err != nil {
			return fmt.Errorf("failed to create sftpusers group: %w", err)
		}
	}

	// Create user and add to sftpusers group
	cmd := exec.Command("useradd", "-m", "-s", "/usr/sbin/nologin", "-G", "sftpusers", username)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create user %s: %w", username, err)
	}

	return nil
}

func renderTemplateWithFuncs(tmplPath string, data DomainData) (string, error) {
	tmpl, err := template.New(filepath.Base(tmplPath)).Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderTemplate(tmplPath string, data DomainData) (string, error) {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	return buf.String(), err
}

func setupDomain(db *storm.DB, domain string, force bool) error {
	var existing DomainRecord
	err := db.One("Domain", domain, &existing)
	if err == nil && !force {
		return fmt.Errorf("domain '%s' already exists", domain)
	}

	if err == nil && force {
		fmt.Println("⚠️ Domain exists, but proceeding due to --force.")
	}

	username := generateUsername(domain)
	if !userExists(username) {
		fmt.Println("Creating user:", username)
		if err := createUnixUser(username); err != nil {
			return fmt.Errorf("could not create user: %w", err)
		}
	}

	if existing.ID == 0 {
		record := DomainRecord{
			Domain:    domain,
			Username:  username,
			CreatedAt: time.Now(),
		}
		if err := db.Save(&record); err != nil {
			return fmt.Errorf("failed to save initial domain record: %w", err)
		}
		existing = record
	}


	outputDir := filepath.Join(viper.GetString("base_output_dir"), domain)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	data := DomainData{
		ID:           existing.ID,
		Domain:       domain,
		DomainDashed: strings.ReplaceAll(domain, ".", "-"),
		PHPVersion:   viper.GetString("default_php_version"),
		Username:     username,
	}

	templates := []string{"docker-compose.yml.tmpl", "php-fpm.Dockerfile.tmpl"}
	for _, tmpl := range templates {
		tmplPath := filepath.Join(viper.GetString("template_dir"), tmpl)
		rendered, err := renderTemplateWithFuncs(tmplPath, data)
		if err != nil {
			return err
		}
		dest := filepath.Join(outputDir, strings.TrimSuffix(tmpl, ".tmpl"))
		if err := os.WriteFile(dest, []byte(rendered), 0644); err != nil {
			return err
		}
		fmt.Println("Generated:", dest)
	}

	domainPath := filepath.Join(websitesRoot, domain)
	jailLink := filepath.Join(jailhomesRoot, username)

	if err := os.MkdirAll(domainPath, 0755); err != nil {
		log.Fatalf("Failed to create domain dir: %v", err)
	}

	if err := os.MkdirAll(jailhomesRoot, 0755); err != nil {
		log.Fatalf("Failed to create jailhomes root dir: %v", err)
	}


	if _, err := os.Lstat(jailLink); os.IsNotExist(err) {
	    if err := os.Symlink(domainPath, jailLink); err != nil {
		    log.Fatalf("Failed to create jail symlink: %v", err)
	    }
	}

	nginxPath := filepath.Join("/etc/nginx/conf.d", fmt.Sprintf("%s.conf", data.DomainDashed))
	tmplPath := filepath.Join(viper.GetString("template_dir"), "nginx.conf.tmpl")
	nginxConf, err := renderTemplateWithFuncs(tmplPath, data)
	if err != nil {
		return fmt.Errorf("failed to render nginx config: %w", err)
	}
	if err := os.WriteFile(nginxPath, []byte(nginxConf), 0644); err != nil {
		return fmt.Errorf("failed to write nginx config: %w", err)
	}
	fmt.Println("✅ Nginx config written to", nginxPath)

	domainDir := filepath.Join(viper.GetString("base_output_dir"), domain)
	if !skipUp {
		if err := runComposeUp(domainDir, runtime, force); err != nil {
			log.Printf("⚠️ Failed to start container: %v", err)
		}
	}

	if !skipReload {
		if err := reloadNginx(); err != nil {
			log.Printf("⚠️ Failed to reload nginx: %v", err)
		}
	}

	if !force {
		record := DomainRecord{
			Domain:    domain,
			Username:  username,
			CreatedAt: time.Now(),
		}
		return db.Save(&record)
	}
	return nil
}

