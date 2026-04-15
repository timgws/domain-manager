package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type DomainData struct {
	ID           int    `storm:"id,increment"`
	Domain       string `storm:"unique"`
	Username     string
	CreatedAt    time.Time
	DomainDashed string
	PHPVersion   string
	UID          int
	GID          int
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
	RunE: func(cmd *cobra.Command, args []string) error {
		setRuntime()
		db := initDB()
		defer db.Close()
		return setupDomain(db, args[0], force)
	},
}

func main() {
	cobra.OnInitialize(initConfig)

	addCmd.Flags().BoolVar(&force, "force", false, "Force overwrite if domain already exists")
	addCmd.Flags().StringVar(&runtime, "runtime", "", "Override container runtime (podman or docker)")
	addCmd.Flags().BoolVar(&skipUp, "no-up", false, "Skip container startup")
	addCmd.Flags().BoolVar(&skipReload, "no-reload", false, "Skip nginx reload")

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

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/domain-manager/")
	viper.AddConfigPath(".")

	viper.SetDefault("nginx_conf_dir", "/etc/nginx/conf.d")
	viper.SetDefault("letsencrypt_live_dir", "/etc/letsencrypt/live")
	viper.SetDefault("letsencrypt_archive_dir", "/etc/letsencrypt/archive")
	viper.SetDefault("letsencrypt_renewal_dir", "/etc/letsencrypt/renewal")
	viper.SetDefault("cloudflare_credentials_dir", "/root/.secrets/cf")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("error reading config: %v", err)
	}

	websitesRoot = viper.GetString("websites_root")
	jailhomesRoot = viper.GetString("jailhomes_root")
	if websitesRoot == "" || jailhomesRoot == "" {
		log.Fatal("websites_root and jailhomes_root must be configured")
	}
}

func initDB() *storm.DB {
	dbPath := viper.GetString("database_path")
	db, err := storm.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open Storm DB: %v", err)
	}
	return db
}

func GetByName(name string) (*DomainData, error) {
	db := initDB()
	defer db.Close()
	return getDomainByName(db, name)
}

func getDomainByName(db *storm.DB, name string) (*DomainData, error) {
	var d DomainData
	if err := db.One("Domain", name, &d); err != nil {
		return nil, fmt.Errorf("domain %s not found: %w", name, err)
	}
	enrichDomain(&d)
	return &d, nil
}

func enrichDomain(d *DomainData) {
	if d.DomainDashed == "" {
		d.DomainDashed = strings.ReplaceAll(d.Domain, ".", "-")
	}
	if d.PHPVersion == "" {
		d.PHPVersion = viper.GetString("default_php_version")
	}
	if d.Username == "" {
		d.Username = generateUsername(d.Domain)
	}
	if d.UID == 0 || d.GID == 0 {
		if usr, err := user.Lookup(d.Username); err == nil {
			if uid, err := strconv.Atoi(usr.Uid); err == nil {
				d.UID = uid
			}
			if gid, err := strconv.Atoi(usr.Gid); err == nil {
				d.GID = gid
			}
		}
	}
}

func setRuntime() {
	if runtime == "" {
		runtime = viper.GetString("default_runtime")
		if runtime == "" {
			runtime = "podman"
		}
	}
	if runtime != "podman" && runtime != "docker" {
		log.Fatalf("invalid runtime: %s (must be 'podman' or 'docker')", runtime)
	}
}

func runComposeUp(domainDir, runtime string, force bool) error {
	var upCmd *exec.Cmd
	if runtime == "docker" {
		upCmd = exec.Command("docker-compose", "up", "--build", "-d")
	} else {
		upCmd = exec.Command("podman-compose", "up", "--build", "-d")
	}

	if force {
		fmt.Printf("🔁 Force mode: bringing down existing %s-compose stack...\n", runtime)
		if err := runComposeDown(domainDir, runtime); err != nil {
			log.Printf("⚠️ failed to run %s-compose down: %v", runtime, err)
		}
	}

	upCmd.Dir = domainDir
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	fmt.Printf("🌀 Starting %s-compose for %s...\n", runtime, domainDir)
	return upCmd.Run()
}

func runComposeDown(domainDir, runtime string) error {
	if _, err := os.Stat(domainDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to access compose directory %s: %w", domainDir, err)
	}

	var downCmd *exec.Cmd
	if runtime == "docker" {
		downCmd = exec.Command("docker-compose", "down")
	} else {
		downCmd = exec.Command("podman-compose", "down")
	}

	downCmd.Dir = domainDir
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	fmt.Printf("🛑 Stopping %s-compose for %s...\n", runtime, domainDir)
	return downCmd.Run()
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
	if err := exec.Command("getent", "group", "sftpusers").Run(); err != nil {
		if err := exec.Command("groupadd", "sftpusers").Run(); err != nil {
			return fmt.Errorf("failed to create sftpusers group: %w", err)
		}
	}

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
	existing, err := getDomainByName(db, domain)
	switch {
	case err == nil && !force:
		return fmt.Errorf("domain %q already exists", domain)
	case err == nil && force:
		fmt.Println("⚠️ Domain exists, but proceeding due to --force.")
	case errors.Is(err, storm.ErrNotFound):
		existing = &DomainData{
			Domain:       domain,
			Username:     generateUsername(domain),
			CreatedAt:    time.Now(),
			DomainDashed: strings.ReplaceAll(domain, ".", "-"),
			PHPVersion:   viper.GetString("default_php_version"),
		}
		if err := db.Save(existing); err != nil {
			return fmt.Errorf("failed to save domain record: %w", err)
		}
	default:
		return err
	}

	if existing.Username == "" {
		existing.Username = generateUsername(domain)
	}
	if !userExists(existing.Username) {
		fmt.Println("Creating user:", existing.Username)
		if err := createUnixUser(existing.Username); err != nil {
			return fmt.Errorf("could not create user: %w", err)
		}
	}
	enrichDomain(existing)
	if existing.UID == 0 || existing.GID == 0 {
		return fmt.Errorf("failed to determine UID/GID for %s", existing.Username)
	}

	outputDir := filepath.Join(viper.GetString("base_output_dir"), domain)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	templates := []string{"docker-compose.yml.tmpl", "php-fpm.Dockerfile.tmpl"}
	for _, tmpl := range templates {
		tmplPath := filepath.Join(viper.GetString("template_dir"), tmpl)
		rendered, err := renderTemplateWithFuncs(tmplPath, *existing)
		if err != nil {
			return err
		}
		dest := filepath.Join(outputDir, strings.TrimSuffix(tmpl, ".tmpl"))
		if err := os.WriteFile(dest, []byte(rendered), 0o644); err != nil {
			return err
		}
		fmt.Println("Generated:", dest)
	}

	domainPath := filepath.Join(websitesRoot, domain)
	jailLink := filepath.Join(jailhomesRoot, existing.Username)
	if err := os.MkdirAll(domainPath, 0o755); err != nil {
		return fmt.Errorf("failed to create domain dir: %w", err)
	}
	if err := os.MkdirAll(jailhomesRoot, 0o755); err != nil {
		return fmt.Errorf("failed to create jailhomes root dir: %w", err)
	}
	if _, err := os.Lstat(jailLink); os.IsNotExist(err) {
		if err := os.Symlink(domainPath, jailLink); err != nil {
			return fmt.Errorf("failed to create jail symlink: %w", err)
		}
	}

	nginxPath := filepath.Join(nginxConfDir(), existing.DomainDashed+".conf")
	tmplPath := filepath.Join(viper.GetString("template_dir"), "nginx.conf.tmpl")
	nginxConf, err := renderTemplateWithFuncs(tmplPath, *existing)
	if err != nil {
		return fmt.Errorf("failed to render nginx config: %w", err)
	}
	if err := os.WriteFile(nginxPath, []byte(nginxConf), 0o644); err != nil {
		return fmt.Errorf("failed to write nginx config: %w", err)
	}
	fmt.Println("✅ Nginx config written to", nginxPath)

	if !skipUp {
		if err := runComposeUp(outputDir, runtime, force); err != nil {
			log.Printf("⚠️ Failed to start container: %v", err)
		}
	}
	if !skipReload {
		if err := reloadNginx(); err != nil {
			log.Printf("⚠️ Failed to reload nginx: %v", err)
		}
	}
	return nil
}

