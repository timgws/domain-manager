package main

import (
	"fmt"
	"strings"
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

func certExists(domain string) bool {
    certPath := filepath.Join("/etc/letsencrypt/live", domain, "fullchain.pem")
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
    args := []string{
        "run", "--webroot", "--nginx",
        "-w", webroot,
        "-d", domain, "-d", "www." + domain,
        "--non-interactive", "--agree-tos",
        "-m", "admin@" + domain,
    }
    return exec.Command("certbot", args...).Run()
}

func runCertbotDNS(domain string) error {
    credPath := filepath.Join("/root/.secrets/cf", domain+".ini")
    if _, err := os.Stat(credPath); os.IsNotExist(err) {
        credPath = "/root/.secrets/cf/default.ini"
    }

    args := []string{
        "run", "--dns-cloudflare", "--nginx",
        "--dns-cloudflare-credentials", credPath,
        "-d", domain, "-d", "www." + domain,
        "--non-interactive", "--agree-tos",
        "-m", "admin@" + domain,
    }
    return exec.Command("certbot", args...).Run()
}

func reloadNginx() error {
	cmd := exec.Command("systemctl", "reload", "nginx")
     cmd.Stdout = os.Stdout
     cmd.Stderr = os.Stderr
     fmt.Println("🔄 Reloading nginx...")
     return cmd.Run()
}
