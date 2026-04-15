package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var forceBootstrap bool

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap [domain] [type]",
	Short: "Bootstrap a Laravel or WordPress project into the domain's web root",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		domainName := args[0]
		kind := strings.ToLower(args[1])

		if kind != "wordpress" && kind != "laravel" {
			return errors.New("invalid type: must be 'wordpress' or 'laravel'")
		}

		d, err := GetByName(domainName)
		if err != nil {
			return err
		}

		return bootstrapProject(d, kind)
	},
}

func bootstrapProject(d *DomainData, kind string) error {
	targetDir := filepath.Join(websitesRoot, d.Domain)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("cannot create domain directory: %w", err)
	}

	// Check if directory exists and is non-empty
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return fmt.Errorf("cannot read domain directory: %w", err)
	}
	if len(entries) > 0 {
		if !forceBootstrap {
			return fmt.Errorf("target directory %s is not empty (use --force to overwrite)", targetDir)
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(targetDir, entry.Name())); err != nil {
				return fmt.Errorf("cannot clear %s: %w", targetDir, err)
			}
		}
	}

	var cmd *exec.Cmd
	if kind == "wordpress" {
		cmd = exec.Command("wp", "core", "download", "--path="+targetDir)
	} else if kind == "laravel" {
		cmd = exec.Command("composer", "create-project", "laravel/laravel", targetDir)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

