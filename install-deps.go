package main

import (
	"crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type osReleaseInfo struct {
	ID        string
	IDLike    string
	VersionID string
}

var installDepsCmd = &cobra.Command{
	Use:   "install-deps",
	Short: "Ensure composer and wp-cli are installed and up-to-date globally",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🔧 Installing/updating dependencies...")

		if err := checkPHP(); err != nil {
			return err
		}
		if err := ensureComposer(); err != nil {
			return fmt.Errorf("composer install failed: %w", err)
		}
		if err := ensureWpCli(); err != nil {
			return fmt.Errorf("wp-cli install failed: %w", err)
		}

		fmt.Println("✅ Dependencies installed successfully.")
		return nil
	},
}

func checkPHP() error {
	if _, err := exec.LookPath("php"); err == nil {
		fmt.Println("✅ PHP is already installed.")
		return nil
	}

	fmt.Println("⚠️ PHP not found. Attempting to install...")

	info, err := readOSRelease()
	if err != nil {
		return err
	}
	if !isRHEL9Compatible(info) {
		return fmt.Errorf("unsupported OS for auto-installation: only RHEL 9-compatible systems are supported")
	}

	bootstrapPackages := [][]string{{
		"dnf", "install", "-y",
		"ca-certificates",
		"curl",
		"gnupg2",
		"dnf-plugins-core",
	}}
	for _, args := range bootstrapPackages {
		if err := runCommand(args[0], args[1:]...); err != nil {
			return err
		}
	}

	stream, err := detectLatestPhpStream()
	if err != nil {
		return err
	}

	if stream == "" {
		fmt.Println("📦 Installing PHP from standard RPM packages...")
		return runCommand("dnf", "install", "-y", "php", "php-fpm", "php-cli", "php-common", "php-mbstring")
	}

	fmt.Printf("📦 Enabling PHP stream: %s\n", stream)
	cmds := [][]string{
		{"dnf", "module", "reset", "-y", "php"},
		{"dnf", "module", "enable", "-y", fmt.Sprintf("php:%s", stream)},
		{"dnf", "install", "-y", "php-fpm", "php-cli", "php-common", "php-mbstring"},
	}
	for _, args := range cmds {
		if err := runCommand(args[0], args[1:]...); err != nil {
			return err
		}
	}

	fmt.Println("✅ PHP installed successfully.")
	return nil
}

func ensureComposer() error {
	if _, err := exec.LookPath("composer"); err == nil {
		fmt.Println("📦 Composer already installed. Updating...")
		return runCommand("composer", "self-update")
	}

	fmt.Println("📦 Composer not found. Installing globally with checksum verification...")
	tmpDir, err := os.MkdirTemp("", "composer-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	installerPath := filepath.Join(tmpDir, "composer-setup.php")
	expectedChecksum, err := downloadText("https://composer.github.io/installer.sig")
	if err != nil {
		return fmt.Errorf("failed to download Composer checksum: %w", err)
	}
	if err := downloadFile("https://getcomposer.org/installer", installerPath); err != nil {
		return fmt.Errorf("failed to download Composer installer: %w", err)
	}

	actualChecksum, err := sha384File(installerPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(expectedChecksum) != actualChecksum {
		return fmt.Errorf("composer installer checksum mismatch")
	}

	return runCommand("php", installerPath, "--install-dir=/usr/local/bin", "--filename=composer")
}

func ensureWpCli() error {
	if _, err := exec.LookPath("wp"); err == nil {
		fmt.Println("📦 WP-CLI already installed. Updating...")
		return runCommand("wp", "cli", "update", "--yes")
	}

	if _, err := exec.LookPath("gpg"); err != nil {
		return fmt.Errorf("gpg is required to verify WP-CLI downloads")
	}

	fmt.Println("📦 WP-CLI not found. Installing globally with GPG verification...")
	tmpDir, err := os.MkdirTemp("", "wpcli-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	pharPath := filepath.Join(tmpDir, "wp-cli.phar")
	ascPath := filepath.Join(tmpDir, "wp-cli.phar.asc")
	keyPath := filepath.Join(tmpDir, "wp-cli.pgp")

	if err := downloadFile("https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar", pharPath); err != nil {
		return fmt.Errorf("failed to download WP-CLI phar: %w", err)
	}
	if err := downloadFile("https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar.asc", ascPath); err != nil {
		return fmt.Errorf("failed to download WP-CLI signature: %w", err)
	}
	if err := downloadFile("https://raw.githubusercontent.com/wp-cli/builds/gh-pages/wp-cli.pgp", keyPath); err != nil {
		return fmt.Errorf("failed to download WP-CLI public key: %w", err)
	}

	gpgHome := filepath.Join(tmpDir, "gpg")
	if err := os.MkdirAll(gpgHome, 0o700); err != nil {
		return err
	}
	if err := runCommand("gpg", "--homedir", gpgHome, "--batch", "--import", keyPath); err != nil {
		return fmt.Errorf("failed to import WP-CLI signing key: %w", err)
	}
	if err := runCommand("gpg", "--homedir", gpgHome, "--batch", "--verify", ascPath, pharPath); err != nil {
		return fmt.Errorf("failed to verify WP-CLI phar: %w", err)
	}

	if err := copyFile(pharPath, "/usr/local/bin/wp", 0o755); err != nil {
		return err
	}
	return nil
}

func detectLatestPhpStream() (string, error) {
	cmd := exec.Command("dnf", "module", "list", "php", "--all")
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	re := regexp.MustCompile(`(?m)^php\s+(\d+)\.(\d+)\s`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	if len(matches) == 0 {
		return "", nil
	}

	type version struct {
		major int
		minor int
		text  string
	}

	versions := make([]version, 0, len(matches))
	for _, match := range matches {
		major, err1 := strconv.Atoi(match[1])
		minor, err2 := strconv.Atoi(match[2])
		if err1 != nil || err2 != nil {
			continue
		}
		versions = append(versions, version{major: major, minor: minor, text: match[1] + "." + match[2]})
	}
	if len(versions) == 0 {
		return "", nil
	}

	sort.Slice(versions, func(i, j int) bool {
		if versions[i].major != versions[j].major {
			return versions[i].major > versions[j].major
		}
		return versions[i].minor > versions[j].minor
	})

	return versions[0].text, nil
}

func readOSRelease() (*osReleaseInfo, error) {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("cannot determine OS: %w", err)
	}

	info := &osReleaseInfo{}
	for _, line := range strings.Split(string(content), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := strings.Trim(parts[1], `"`)
		switch key {
		case "ID":
			info.ID = value
		case "ID_LIKE":
			info.IDLike = value
		case "VERSION_ID":
			info.VersionID = value
		}
	}
	return info, nil
}

func isRHEL9Compatible(info *osReleaseInfo) bool {
	major := strings.SplitN(info.VersionID, ".", 2)[0]
	if major != "9" {
		return false
	}

	ids := append([]string{info.ID}, strings.Fields(info.IDLike)...)
	for _, id := range ids {
		switch id {
		case "rhel", "rocky", "almalinux":
			return true
		}
	}
	return false
}

func downloadText(url string) (string, error) {
	var httpClient = &http.Client{
		Timeout: 60 * time.Second * 10,
	}

	resp, err := httpClient.Get(url)

	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected HTTP status %s for %s", resp.Status, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func downloadFile(url, path string) error {
	var httpClient = &http.Client{
		Timeout: 60 * time.Second * 10,
	}

	resp, err := httpClient.Get(url)

	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %s for %s", resp.Status, url)
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func sha384File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha512.New384()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(mode)
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
