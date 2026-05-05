# domain-manager

**Domain Manager** is a lightweight CLI tool written in Go to help you self-host multiple PHP-based websites — especially WordPress or Laravel — using per-domain `php-fpm` containers, backed by Docker or Podman. It's designed for a simplified setup where nginx and MySQL run directly on the host, and each site gets its own isolated PHP runtime. The tool automates all scaffolding: directory layout, container config, MySQL database creation, and nginx integration. Ideal for developers who want to self-host multiple sites with minimal overhead, full control, and no panel bloat.

---

## Why Use domain-manager Instead of cPanel?

Traditional hosting panels are heavy, opinionated, and often come with licensing costs, background services, and bundled features you may never use. `domain-manager` is built for developers and sysadmins who want:

* 🧱 **Full control** — No hidden automation or black-box behavior
* 🐧 **Linux-native** — Works seamlessly with your system's users, files, and firewalls
* 🐳 **Container-based isolation** — Each site runs its own PHP-FPM container for clean separation
* 🔍 **Transparent configuration** — Every file (nginx, Docker, MySQL) is visible and editable
* 📦 **DevOps-friendly** — Easily integrates with GitOps, systemd, and shell automation

`domain-manager` is ideal for minimal, scalable, modern PHP hosting — no control panel bloat required.

---

## ✨ Features

- 🔧 Per-domain `php-fpm` container generation (Docker/Podman)
- 📂 Mount WordPress files from `/data/websites/{domain}`
- 💾 Per-domain backup command that creates a `.tar.bz2` archive
- 🐧 Creates isolated Unix system user per domain
- 🐬 MySQL database provisioning with namespaced DBs (e.g. `example_com_main`)
- 🔑 Auto-generated secure MySQL passwords (displayed once)
- 🔌 nginx configuration with FastCGI routing
- ⚙️ Configurable via `config.yaml` (powered by `spf13/viper`)
- 📋 Domain and DB state tracking with `storm` (BoltDB wrapper)
- 🧪 Lifecycle commands: `add`, `list`, `info`, `delete`
- 🚀 Supports `--runtime`, `--force`, `--no-up`, `--no-reload` flags

---

## 🛠 Requirements

- Go 1.20+
- Podman or Docker
- nginx installed and running on the host
- MySQL server running on the host
- Directory structure: `/data/websites/{domain}`

---

## 🐬 Installing MySQL (Percona Server 8.4 LTS)
To keep your setup lean, Domain Manager assumes that **MySQL is running on the host**, not in a container. For maximum compatibility and long-term support, we recommend using Percona Server for [MySQL 8.4 LTS](https://www.percona.com/software/mysql-database/percona-server-for-mysql), a drop-in replacement for MySQL with extended support and observability features.

Here's how to install it on RHEL-based systems (e.g. AlmaLinux, Rocky Linux):

```
dnf install https://repo.percona.com/yum/percona-release-latest.noarch.rpm
percona-release enable pdps-84-lts
dnf install percona-server-server percona-server-client

# Disable telemetry
systemctl stop percona-telemetry-agent
systemctl mask percona-telemetry-agent

systemctl enable --now mysqld
```

Make sure that you 

```
CREATE USER 'root'@'127.0.0.1' IDENTIFIED BY ')AS8SDA(sgd89a98';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1' WITH GRANT OPTION;
FLUSH PRIVILEGES;
```

---

## Allowing users to SFTP files
Add the SFTP config from `system-config/sshd_config` to `/etc/sshd_config` to ensure that users are jailed to `jailhomes`.

Either give users access to SFTP with a password using `passwd <user>` (not recommended) or set them up with an SSH key.

---

## 📦 Install

```bash
git clone https://github.com/timgws/domain-manager.git
cd domain-manager
go build -o domain-manager .
```

## 🚀 Usage
```sh
# List all of the domains that we are managing
./domain-manager list

# Add, find info, and delete domains
./domain-manager add example.com
./domain-manager info example.com
./domain-manager delete example.com

# Manage MySQL databases
./domain-manager mysql create example.com example_db_name
./domain-manager mysql list example.com --stats

# SSL
./domain-manager enable-ssl example.com --cloudflare
```

## Backups

The `backup` command creates a `.tar.bz2` archive containing:

- `site/` — the website root for the domain
- `databases/` — SQL dumps for all MySQL databases associated with the domain
- `metadata.txt` — basic domain backup metadata

Example:

```bash
./domain-manager backup example.com --output-dir /backups
```

### Flags
* `--runtime docker|podman` — override container runtime
* `--force` — recreate existing container (runs down before up)
* `--no-up` — skip container startup
* `--no-reload` — skip nginx reload

## Troubleshooting
### Setting up servers for reboots
```
# Check if podman has set the restart policy away from 'no'.
podman inspect php-example-org --format '{{.HostConfig.RestartPolicy.Name}}'

# Change the restart policy to unless-stopped.
podman update --restart=unless-stopped php-example-org
```

### Make sure the PHP version is correct
When you are trying to confirm a PHP upgrade, check both:
1. the PHP version inside the running container
2. the PHP version in the current image tag

This tells you what version the currently running container is actually using:
```
podman exec php-example-org php -v
```

This tells you what version the latest built image contains:
```
podman run --rm localhost/exampleorg_php:latest php -v
```

If these are different, the problem is that the existing container was
created from the older PHP image and is still being started again at boot.

To apply the new PHP version, you need to remove the old container and
recreate it from the rebuilt image.

```
podman-compose down
podman rm -f php-example-org
podman-compose up --build -d
```

* rebuilding the image updates `localhost/exampleorg_php:latest`
* it does not automatically upgrade an already-existing container
* removing and recreating the container ensures the running container actually uses the new PHP version

## 🧱 Stack
* `nginx` serves static files and proxies `.php` requests to host-mapped container ports
* `PHP-FPM` runs per-site in containers
* MySQL is shared across all sites

## 📁 Directory Structure
```
/opt/domain-manager/docker/example.com/
  ├── Dockerfile (from template)
  └── docker-compose.yaml

/etc/nginx/conf.d/example-com.conf

/data/websites/example.com/
```

## 📚 License

MIT License — use it, modify it, fork it.
