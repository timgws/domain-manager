package main

import (
	"fmt"
	"os"
	"os/exec"
)

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
			return fmt.Errorf("failed to run %s-compose down before force rebuild: %w", runtime, err)
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

