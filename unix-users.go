package main

import (
	"fmt"
	"os/exec"
)

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

