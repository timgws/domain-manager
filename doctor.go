package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type doctorStatus string

const (
	doctorPass doctorStatus = "pass"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
	doctorSkip doctorStatus = "skip"
)

type doctorCheck struct {
	Status doctorStatus
	Name   string
	Detail string
}

type doctorReport struct {
	Checks []doctorCheck
}

var doctorCmd = &cobra.Command{
	Use:   "doctor [domain]",
	Short: "Run health checks for the domain-manager host or a specific domain",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDoctorCommand,
}

func runDoctorCommand(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	report := &doctorReport{}

	runGlobalDoctorChecks(ctx, report)

	if len(args) == 1 {
		runDomainDoctorChecks(ctx, report, canonicalDomain(args[0]))
	}

	report.Print()

	if report.FailureCount() > 0 {
		return fmt.Errorf("doctor found %d failing check(s)", report.FailureCount())
	}

	return nil
}

func runGlobalDoctorChecks(ctx context.Context, report *doctorReport) {
	checkConfig(report)
	checkRequiredDirectories(report)
	checkStormDatabase(report)
	checkRequiredBinaries(ctx, report)
	checkNginx(ctx, report)
	checkMySQL(ctx, report)
	checkTemplates(report)
}

func runDomainDoctorChecks(ctx context.Context, report *doctorReport, domain string) {
	report.Add(doctorSkip, "Domain checks", fmt.Sprintf("checking %s", domain))

	if err := validateDomainName(domain); err != nil {
		report.Add(doctorFail, "Domain name", err.Error())
		return
	}

	db, err := openStormDBForDoctor()
	if err != nil {
		report.Add(doctorFail, "Domain database", err.Error())
		return
	}
	defer db.Close()

	d, err := getDomainByName(db, domain)
	if err != nil {
		report.Add(doctorFail, "Domain record", err.Error())
		return
	}

	report.Add(doctorPass, "Domain record", fmt.Sprintf("found ID %d for %s", d.ID, d.Domain))

	checkDomainFilesystem(report, d)
	checkDomainNginxConfig(report, d)
	checkDomainComposeFiles(report, d)
	checkDomainMySQLRecords(report, db, d)
	checkDomainContainer(ctx, report, d)
	checkDomainPHPPort(report, d)
	checkDomainCertificate(report, d)
	checkDomainHTTP(ctx, report, d)
}

func checkConfig(report *doctorReport) {
	configPath := viper.ConfigFileUsed()
	if configPath == "" {
		report.Add(doctorFail, "Config file", "no config file loaded")
	} else {
		report.Add(doctorPass, "Config file", configPath)
	}

	required := map[string]string{
		"database_path":    viper.GetString("database_path"),
		"websites_root":    websitesRoot,
		"jailhomes_root":   jailhomesRoot,
		"base_output_dir":  viper.GetString("base_output_dir"),
		"template_dir":     viper.GetString("template_dir"),
		"nginx_conf_dir":   nginxConfDir(),
		"default_runtime":  viper.GetString("default_runtime"),
		"mysql.host":       viper.GetString("mysql.host"),
		"mysql.root_user":  viper.GetString("mysql.root_user"),
	}

	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			report.Add(doctorFail, "Config "+key, "missing or empty")
			continue
		}

		report.Add(doctorPass, "Config "+key, value)
	}

	if viper.GetString("mysql.root_password") == "" {
		report.Add(doctorWarn, "Config mysql.root_password", "empty password configured")
	} else {
		report.Add(doctorPass, "Config mysql.root_password", "configured")
	}
}

func checkRequiredDirectories(report *doctorReport) {
	checkDirectory(report, "Websites root", websitesRoot, true)
	checkDirectory(report, "Jailhomes root", jailhomesRoot, true)
	checkDirectory(report, "Base output directory", viper.GetString("base_output_dir"), true)
	checkDirectory(report, "Template directory", viper.GetString("template_dir"), false)
	checkDirectory(report, "nginx config directory", nginxConfDir(), true)

	dbPath := viper.GetString("database_path")
	if dbPath != "" {
		checkDirectory(report, "Database parent directory", filepath.Dir(dbPath), true)
	}
}

func checkDirectory(report *doctorReport, name, path string, requireWritable bool) {
	if strings.TrimSpace(path) == "" {
		report.Add(doctorFail, name, "path is not configured")
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			report.Add(doctorFail, name, fmt.Sprintf("%s does not exist", path))
			return
		}

		report.Add(doctorFail, name, fmt.Sprintf("cannot stat %s: %v", path, err))
		return
	}

	if !info.IsDir() {
		report.Add(doctorFail, name, fmt.Sprintf("%s is not a directory", path))
		return
	}

	if !requireWritable {
		report.Add(doctorPass, name, path)
		return
	}

	tmp, err := os.CreateTemp(path, ".domain-manager-doctor-*")
	if err != nil {
		report.Add(doctorFail, name, fmt.Sprintf("%s exists but is not writable: %v", path, err))
		return
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(tmpName)

	report.Add(doctorPass, name, fmt.Sprintf("%s exists and is writable", path))
}

func checkStormDatabase(report *doctorReport) {
	dbPath := viper.GetString("database_path")
	if strings.TrimSpace(dbPath) == "" {
		report.Add(doctorFail, "Storm database", "database_path is not configured")
		return
	}

	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			report.Add(doctorWarn, "Storm database file", fmt.Sprintf("%s does not exist yet", dbPath))
		} else {
			report.Add(doctorFail, "Storm database file", fmt.Sprintf("cannot stat %s: %v", dbPath, err))
			return
		}
	} else {
		report.Add(doctorPass, "Storm database file", dbPath)
	}

	db, err := storm.Open(dbPath)
	if err != nil {
		report.Add(doctorFail, "Storm database open", err.Error())
		return
	}
	defer db.Close()

	var domains []DomainData
	if err := db.All(&domains); err != nil {
		if errors.Is(err, storm.ErrNotFound) {
			report.Add(doctorWarn, "Storm DomainData bucket", "no domains have been created yet")
			return
		}

		report.Add(doctorFail, "Storm DomainData bucket", err.Error())
		return
	}

	report.Add(doctorPass, "Storm DomainData bucket", fmt.Sprintf("%d domain(s) tracked", len(domains)))
}

func checkRequiredBinaries(ctx context.Context, report *doctorReport) {
	checkBinary(report, "nginx", "nginx")
	checkBinary(report, "certbot", "certbot")
	checkBinary(report, "tar", "tar")
	checkBinary(report, "mysqldump", "mysqldump")
	checkBinary(report, "mysql client", "mysql")
	checkBinary(report, "systemctl", "systemctl")

	resolvedRuntime, err := resolveDoctorRuntime()
	if err != nil {
		report.Add(doctorFail, "Container runtime", err.Error())
		return
	}

	report.Add(doctorPass, "Container runtime", resolvedRuntime)

	switch resolvedRuntime {
	case "podman":
		checkBinary(report, "podman", "podman")
		checkBinary(report, "podman-compose", "podman-compose")

	case "docker":
		checkBinary(report, "docker", "docker")
		checkDockerCompose(ctx, report)
	}
}

func checkBinary(report *doctorReport, name, binary string) {
	path, err := exec.LookPath(binary)
	if err != nil {
		report.Add(doctorFail, name, fmt.Sprintf("%s not found in PATH", binary))
		return
	}

	report.Add(doctorPass, name, path)
}

func checkDockerCompose(ctx context.Context, report *doctorReport) {
	if path, err := exec.LookPath("docker-compose"); err == nil {
		report.Add(doctorPass, "docker-compose", path)
		return
	}

	out, err := runCommandOutput(ctx, 5*time.Second, "docker", "compose", "version")
	if err != nil {
		report.Add(doctorFail, "Docker Compose", summarizeOutput(out, err))
		return
	}

	report.Add(doctorPass, "Docker Compose", strings.TrimSpace(string(out)))
}

func checkNginx(ctx context.Context, report *doctorReport) {
	if _, err := exec.LookPath("nginx"); err != nil {
		report.Add(doctorFail, "nginx config test", "nginx not found in PATH")
		return
	}

	out, err := runCommandOutput(ctx, 10*time.Second, "nginx", "-t")
	if err != nil {
		report.Add(doctorFail, "nginx config test", summarizeOutput(out, err))
		return
	}

	report.Add(doctorPass, "nginx config test", strings.TrimSpace(string(out)))

	if _, err := exec.LookPath("systemctl"); err != nil {
		report.Add(doctorSkip, "nginx service", "systemctl not found")
		return
	}

	out, err = runCommandOutput(ctx, 5*time.Second, "systemctl", "is-active", "nginx")
	if err != nil {
		report.Add(doctorFail, "nginx service", summarizeOutput(out, err))
		return
	}

	status := strings.TrimSpace(string(out))
	if status != "active" {
		report.Add(doctorFail, "nginx service", status)
		return
	}

	report.Add(doctorPass, "nginx service", "active")
}

func checkMySQL(ctx context.Context, report *doctorReport) {
	mysql, err := connectToMySQL()
	if err != nil {
		report.Add(doctorFail, "MySQL connection", err.Error())
		return
	}
	defer mysql.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := mysql.PingContext(pingCtx); err != nil {
		report.Add(doctorFail, "MySQL connection", err.Error())
		return
	}

	report.Add(doctorPass, "MySQL connection", "connected successfully")
}

func checkTemplates(report *doctorReport) {
	templateDir := viper.GetString("template_dir")
	if templateDir == "" {
		report.Add(doctorFail, "Templates", "template_dir is not configured")
		return
	}

	requiredTemplates := []string{
		"nginx.conf.tmpl",
		"docker-compose.yml.tmpl",
		"php-fpm.Dockerfile.tmpl",
	}

	for _, name := range requiredTemplates {
		path := filepath.Join(templateDir, name)

		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				report.Add(doctorFail, "Template "+name, "missing")
				continue
			}

			report.Add(doctorFail, "Template "+name, err.Error())
			continue
		}

		if err := parseTemplateForDoctor(path); err != nil {
			report.Add(doctorFail, "Template "+name, err.Error())
			continue
		}

		report.Add(doctorPass, "Template "+name, path)
	}
}

func parseTemplateForDoctor(path string) error {
	_, err := template.New(filepath.Base(path)).Funcs(template.FuncMap{
		"add":  func(a, b int) int { return a + b },
		"join": strings.Join,
	}).ParseFiles(path)

	return err
}

func checkDomainFilesystem(report *doctorReport, d *DomainData) {
	siteRoot := filepath.Join(websitesRoot, d.Domain)
	checkDirectory(report, "Domain website root", siteRoot, false)

	jailLink := filepath.Join(jailhomesRoot, d.Username)

	info, err := os.Lstat(jailLink)
	if err != nil {
		if os.IsNotExist(err) {
			report.Add(doctorFail, "Domain jail link", fmt.Sprintf("%s does not exist", jailLink))
			return
		}

		report.Add(doctorFail, "Domain jail link", err.Error())
		return
	}

	if info.Mode()&os.ModeSymlink == 0 {
		report.Add(doctorFail, "Domain jail link", fmt.Sprintf("%s is not a symlink", jailLink))
		return
	}

	target, err := os.Readlink(jailLink)
	if err != nil {
		report.Add(doctorFail, "Domain jail link", err.Error())
		return
	}

	expected := filepath.Join(websitesRoot, d.Domain)
	if target != expected {
		report.Add(doctorWarn, "Domain jail link", fmt.Sprintf("points to %s, expected %s", target, expected))
		return
	}

	report.Add(doctorPass, "Domain jail link", fmt.Sprintf("%s -> %s", jailLink, target))
}

func checkDomainNginxConfig(report *doctorReport, d *DomainData) {
	nginxPath := filepath.Join(nginxConfDir(), d.DomainDashed+".conf")

	if _, err := os.Stat(nginxPath); err != nil {
		if os.IsNotExist(err) {
			report.Add(doctorFail, "Domain nginx config", fmt.Sprintf("%s does not exist", nginxPath))
			return
		}

		report.Add(doctorFail, "Domain nginx config", err.Error())
		return
	}

	report.Add(doctorPass, "Domain nginx config", nginxPath)
}

func checkDomainComposeFiles(report *doctorReport, d *DomainData) {
	composeDir := filepath.Join(viper.GetString("base_output_dir"), d.Domain)
	checkDirectory(report, "Domain compose directory", composeDir, false)

	composeFiles := []string{
		"docker-compose.yml",
		"php-fpm.Dockerfile",
	}

	for _, name := range composeFiles {
		path := filepath.Join(composeDir, name)

		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				report.Add(doctorFail, "Domain "+name, "missing")
				continue
			}

			report.Add(doctorFail, "Domain "+name, err.Error())
			continue
		}

		report.Add(doctorPass, "Domain "+name, path)
	}
}

func checkDomainMySQLRecords(report *doctorReport, db *storm.DB, d *DomainData) {
	var records []MySQLDatabaseRecord

	if err := db.Find("Domain", d.Domain, &records); err != nil {
		if errors.Is(err, storm.ErrNotFound) {
			report.Add(doctorPass, "Domain MySQL records", "no databases tracked")
			return
		}

		report.Add(doctorFail, "Domain MySQL records", err.Error())
		return
	}

	report.Add(doctorPass, "Domain MySQL records", fmt.Sprintf("%d database(s) tracked", len(records)))
}

func checkDomainContainer(ctx context.Context, report *doctorReport, d *DomainData) {
	resolvedRuntime, err := resolveDoctorRuntime()
	if err != nil {
		report.Add(doctorFail, "Domain container", err.Error())
		return
	}

	if _, err := exec.LookPath(resolvedRuntime); err != nil {
		report.Add(doctorSkip, "Domain container", fmt.Sprintf("%s not found", resolvedRuntime))
		return
	}

	containerName := "php-" + d.DomainDashed

	out, err := runCommandOutput(ctx, 5*time.Second, resolvedRuntime, "inspect", "-f", "{{.State.Running}}", containerName)
	if err != nil {
		report.Add(doctorFail, "Domain container", fmt.Sprintf("%s not found or not inspectable: %s", containerName, summarizeOutput(out, err)))
		return
	}

	running := strings.TrimSpace(string(out))
	if running != "true" {
		report.Add(doctorFail, "Domain container", fmt.Sprintf("%s running=%s", containerName, running))
		return
	}

	report.Add(doctorPass, "Domain container", fmt.Sprintf("%s is running", containerName))
}

func checkDomainPHPPort(report *doctorReport, d *DomainData) {
	port := 9000 + d.ID
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		report.Add(doctorFail, "Domain PHP-FPM port", fmt.Sprintf("%s is not reachable: %v", addr, err))
		return
	}
	_ = conn.Close()

	report.Add(doctorPass, "Domain PHP-FPM port", fmt.Sprintf("%s is listening", addr))
}

func checkDomainCertificate(report *doctorReport, d *DomainData) {
	certPath := filepath.Join(viper.GetString("letsencrypt_live_dir"), d.Domain, "fullchain.pem")

	if _, err := os.Stat(certPath); err != nil {
		if os.IsNotExist(err) {
			report.Add(doctorWarn, "Domain SSL certificate", fmt.Sprintf("%s does not exist", certPath))
			return
		}

		report.Add(doctorFail, "Domain SSL certificate", err.Error())
		return
	}

	cert, err := readFirstCertificate(certPath)
	if err != nil {
		report.Add(doctorFail, "Domain SSL certificate", err.Error())
		return
	}

	if err := cert.VerifyHostname(d.Domain); err != nil {
		report.Add(doctorFail, "Domain SSL certificate hostname", err.Error())
		return
	}

	now := time.Now()

	if now.After(cert.NotAfter) {
		report.Add(doctorFail, "Domain SSL certificate expiry", fmt.Sprintf("expired at %s", cert.NotAfter.Format(time.RFC3339)))
		return
	}

	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

	if daysLeft < 30 {
		report.Add(doctorWarn, "Domain SSL certificate expiry", fmt.Sprintf("expires in %d day(s) at %s", daysLeft, cert.NotAfter.Format(time.RFC3339)))
		return
	}

	report.Add(doctorPass, "Domain SSL certificate", fmt.Sprintf("valid for %s, expires in %d day(s)", d.Domain, daysLeft))
}

func checkDomainHTTP(ctx context.Context, report *doctorReport, d *DomainData) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://127.0.0.1/", nil)
	if err != nil {
		report.Add(doctorFail, "Domain HTTP check", err.Error())
		return
	}

	req.Host = d.Domain

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		report.Add(doctorWarn, "Domain HTTP check", fmt.Sprintf("request failed for Host %s: %v", d.Domain, err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		report.Add(doctorFail, "Domain HTTP check", fmt.Sprintf("HTTP %d from nginx for Host %s", resp.StatusCode, d.Domain))
		return
	}

	report.Add(doctorPass, "Domain HTTP check", fmt.Sprintf("HTTP %d from nginx for Host %s", resp.StatusCode, d.Domain))
}

func readFirstCertificate(path string) (*x509.Certificate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return nil, fmt.Errorf("no PEM certificate found in %s", path)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

func resolveDoctorRuntime() (string, error) {
	resolved := runtime

	if resolved == "" {
		resolved = viper.GetString("default_runtime")
	}

	if resolved == "" {
		resolved = "podman"
	}

	if resolved != "podman" && resolved != "docker" {
		return "", fmt.Errorf("invalid runtime %q; must be 'podman' or 'docker'", resolved)
	}

	return resolved, nil
}

func openStormDBForDoctor() (*storm.DB, error) {
	dbPath := viper.GetString("database_path")
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("database_path is not configured")
	}

	db, err := storm.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Storm DB %s: %w", dbPath, err)
	}

	return db, nil
}

func runCommandOutput(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.CombinedOutput()

	if cmdCtx.Err() != nil {
		return out, cmdCtx.Err()
	}

	return out, err
}

func summarizeOutput(out []byte, err error) string {
	text := strings.TrimSpace(string(out))

	if text == "" {
		return err.Error()
	}

	text = strings.ReplaceAll(text, "\n", "; ")

	if len(text) > 300 {
		text = text[:300] + "..."
	}

	return fmt.Sprintf("%v: %s", err, text)
}

func (r *doctorReport) Add(status doctorStatus, name, detail string) {
	r.Checks = append(r.Checks, doctorCheck{
		Status: status,
		Name:   name,
		Detail: detail,
	})
}

func (r *doctorReport) FailureCount() int {
	count := 0

	for _, check := range r.Checks {
		if check.Status == doctorFail {
			count++
		}
	}

	return count
}

func (r *doctorReport) WarningCount() int {
	count := 0

	for _, check := range r.Checks {
		if check.Status == doctorWarn {
			count++
		}
	}

	return count
}

func (r *doctorReport) Print() {
	fmt.Println("🩺 domain-manager doctor")
	fmt.Println()

	for _, check := range r.Checks {
		icon := "•"

		switch check.Status {
		case doctorPass:
			icon = "✅"
		case doctorWarn:
			icon = "⚠️ "
		case doctorFail:
			icon = "❌"
		case doctorSkip:
			icon = "➖"
		}

		if check.Detail == "" {
			fmt.Printf("%s %-6s %s\n", icon, strings.ToUpper(string(check.Status)), check.Name)
			continue
		}

		fmt.Printf("%s %-6s %s — %s\n", icon, strings.ToUpper(string(check.Status)), check.Name, check.Detail)
	}

	fmt.Println()
	fmt.Printf("Summary: %d check(s), %d warning(s), %d failure(s)\n", len(r.Checks), r.WarningCount(), r.FailureCount())
}

