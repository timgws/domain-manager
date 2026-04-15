package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

func promptYesNo(reader *bufio.Reader, prompt string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}

	fmt.Printf("%s %s: ", prompt, suffix)

	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}

	answer := strings.ToLower(strings.TrimSpace(input))
	switch answer {
	case "":
		return defaultYes, nil
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid response %q", answer)
	}
}

func certExists(domain string) bool {
	certPath := filepath.Join(viper.GetString("letsencrypt_live_dir"), domain, "fullchain.pem")
	_, err := os.Stat(certPath)
	return !os.IsNotExist(err)
}

func autoDetectCloudflare(domain string) bool {
	ns, err := net.LookupNS(domain)
	if err != nil {
		return false
	}
	for _, n := range ns {
		if strings.Contains(n.Host, "cloudflare.com") {
			return true
		}
	}
	return false
}

func runCertbotWebroot(domain, webroot string) error {
	email := viper.GetString("certbot_email")
	if email == "" {
		email = "admin@" + domain
	}

	args := []string{
		"run", "--webroot", "--nginx",
		"-w", webroot,
		"-d", domain, "-d", "www." + domain,
		"--non-interactive", "--agree-tos",
		"-m", email,
	}
	cmd := exec.Command("certbot", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCertbotDNS(domain string) error {
	credPath := filepath.Join(viper.GetString("cloudflare_credentials_dir"), domain+".ini")
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		credPath = filepath.Join(viper.GetString("cloudflare_credentials_dir"), "default.ini")
	}

	email := viper.GetString("certbot_email")
	if email == "" {
		email = "admin@" + domain
	}

	args := []string{
		"run", "--dns-cloudflare", "--nginx",
		"--dns-cloudflare-credentials", credPath,
		"-d", domain, "-d", "www." + domain,
		"--non-interactive", "--agree-tos",
		"-m", email,
	}
	cmd := exec.Command("certbot", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func reloadNginx() error {
	cmd := exec.Command("systemctl", "reload", "nginx")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println("🔄 Reloading nginx...")
	return cmd.Run()
}

func nginxConfDir() string {
	return viper.GetString("nginx_conf_dir")
}
