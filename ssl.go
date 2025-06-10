package main

import (
	"log"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var enableSSL = &cobra.Command{
    Use:   "enable-ssl [domain]",
    Short: "Enable Let's Encrypt SSL for a domain",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        domain := args[0]
        domainDir := filepath.Join("/data/websites", domain)
        cfFlag, _ := cmd.Flags().GetBool("cloudflare")
        noReload, _ := cmd.Flags().GetBool("no-reload")

        isCloudflare := cfFlag || autoDetectCloudflare(domain)

        if certExists(domain) {
            fmt.Println("✅ SSL already enabled.")
            return
        }

        var err error
        if isCloudflare {
            err = runCertbotDNS(domain)
        } else {
            err = runCertbotWebroot(domain, domainDir)
        }

        if err != nil {
            log.Fatalf("❌ Failed to enable SSL: %v", err)
        }

        if !noReload {
            reloadNginx()
        }
    },
}

