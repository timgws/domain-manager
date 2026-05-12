package main

import (
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/asdine/storm/v3"
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

func GetByName(name string) (*DomainData, error) {
	db := initDB()
	defer db.Close()

	return getDomainByName(db, canonicalDomain(name))
}

func getDomainByName(db *storm.DB, name string) (*DomainData, error) {
	var d DomainData

	name = canonicalDomain(name)

	if err := db.One("Domain", name, &d); err != nil {
		return nil, fmt.Errorf("domain %s not found: %w", name, err)
	}

	enrichDomain(&d)

	return &d, nil
}

func enrichDomain(d *DomainData) {
	if d.Domain != "" {
		d.Domain = canonicalDomain(d.Domain)
	}

	if d.DomainDashed == "" {
		d.DomainDashed = dashedDomain(d.Domain)
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

func generateUsername(domain string) string {
	base := "wp_" + strings.ReplaceAll(canonicalDomain(domain), ".", "_")

	if len(base) > 30 {
		base = base[:30]
	}

	return base
}
