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
To keep your setup lean, Domain Manager assumes that **MySQL is running on the host**, not in a container. For maximum compatibility and long-term support, we recommend using Percona Server for [MySQL 8.4 LTS][https://www.percona.com/software/mysql-database/percona-server-for-mysql], a drop-in replacement for MySQL with extended support and observability features.

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
```

### Flags
* `--runtime docker|podman` — override container runtime
* `--force` — recreate existing container (runs down before up)
* `--no-up` — skip container startup
* `--no-reload` — skip nginx reload

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
