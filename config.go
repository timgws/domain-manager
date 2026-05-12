package main

import (
	"log"

	"github.com/asdine/storm/v3"
	"github.com/spf13/viper"
)

var (
	websitesRoot  string
	jailhomesRoot string
)

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/domain-manager/")
	viper.AddConfigPath(".")

	viper.SetDefault("nginx_conf_dir", "/etc/nginx/conf.d")
	viper.SetDefault("letsencrypt_live_dir", "/etc/letsencrypt/live")
	viper.SetDefault("letsencrypt_archive_dir", "/etc/letsencrypt/archive")
	viper.SetDefault("letsencrypt_renewal_dir", "/etc/letsencrypt/renewal")
	viper.SetDefault("cloudflare_credentials_dir", "/root/.secrets/cf")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("error reading config: %v", err)
	}

	websitesRoot = viper.GetString("websites_root")
	jailhomesRoot = viper.GetString("jailhomes_root")

	if websitesRoot == "" || jailhomesRoot == "" {
		log.Fatal("websites_root and jailhomes_root must be configured")
	}
}

func initDB() *storm.DB {
	dbPath := viper.GetString("database_path")

	db, err := storm.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open Storm DB: %v", err)
	}

	return db
}

func setRuntime() {
	if runtime == "" {
		runtime = viper.GetString("default_runtime")
		if runtime == "" {
			runtime = "podman"
		}
	}

	if runtime != "podman" && runtime != "docker" {
		log.Fatalf("invalid runtime: %s (must be 'podman' or 'docker')", runtime)
	}
}
