package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"regexp"

	"github.com/spf13/cobra"
)

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
	_, err := exec.LookPath("php")
	if err == nil {
		fmt.Println("✅ PHP is already installed.")
		return nil
	}

	fmt.Println("⚠️ PHP not found. Attempting to install...")

	// Check if we're on Rocky Linux 9
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("cannot determine OS: %w", err)
	}
	osInfo := string(content)
	if !(strings.Contains(osInfo, "Rocky Linux") && strings.Contains(osInfo, `VERSION_ID="9.6"`)) {
		return fmt.Errorf("unsupported OS for auto-installation: only Rocky Linux 9 supported")
	}

	phpStream, err := detectLatestPhpStream()
		if err != nil {
			return err
		}

	fmt.Printf("📦 Enabling PHP stream: %s\n", phpStream)

	fmt.Println("📦 Installing PHP from AppStream module...")
	cmds := [][]string{
		{"dnf", "install", "-y", "dnf-plugins-core"},
		{"dnf", "module", "reset", "-y", "php"},
		{"dnf", "module", "enable", "-y", fmt.Sprintf("php:%s", phpStream)},
		{"dnf", "install", "-y", "php-fpm", "php-cli", "php-common", "php-mbstring"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed running: %v: %w", args, err)
		}
	}

	fmt.Println("✅ PHP installed successfully via AppStream.")
	return nil
}

func ensureComposer() error {
	_, err := exec.LookPath("composer")
	if err != nil {
		fmt.Println("📦 Composer not found. Installing globally...")
		// Download and install composer
		cmd := exec.Command("php", "-r", "copy('https://getcomposer.org/installer', 'composer-setup.php');")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		cmd = exec.Command("php", "composer-setup.php", "--install-dir=/usr/local/bin", "--filename=composer")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		_ = os.Remove("composer-setup.php")
	} else {
		fmt.Println("📦 Composer already installed. Updating...")
		cmd := exec.Command("composer", "self-update")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func ensureWpCli() error {
	_, err := exec.LookPath("wp")
	if err != nil {
		fmt.Println("📦 WP-CLI not found. Installing globally...")
		// Download and install wp-cli
		cmd := exec.Command("curl", "-O", "https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		cmd = exec.Command("chmod", "+x", "wp-cli.phar")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		cmd = exec.Command("mv", "wp-cli.phar", "/usr/local/bin/wp")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		fmt.Println("📦 WP-CLI already installed.")
	}
	return nil
}

func detectLatestPhpStream() (string, error) {
	cmd := exec.Command("dnf", "module", "list", "php", "--all")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list php module streams: %w", err)
	}

	re := regexp.MustCompile(`(?m)^php\s+(\d\.\d)\s`)
	var latest string
	for _, match := range re.FindAllStringSubmatch(string(out), -1) {
		if len(match) > 1 {
			latest = match[1]
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no php module stream found")
	}
	return latest, nil
}
