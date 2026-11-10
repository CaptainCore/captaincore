#!/bin/bash
# CaptainCore site image: Debian 13 with FrankenPHP (one PHP branch), MariaDB,
# sshd and WP-CLI. Runs as root inside a fresh images:debian/13 container;
# `captaincore node image build <server> --php=8.x` does that and publishes
# the result as cc-wp-php8x. Embedded into the CLI by cmd/node_image.go.
#
# PHP_BRANCH picks the package repository (83, 84, 85): pkg.henderkes.com
# keeps one per PHP branch with the same package names, so that one variable
# is the only difference between the images. Everything a site can change
# later (PHP settings, the web server rules, hardening) is written again by
# `host apply`; what is here are the defaults a new container boots with.
set -e
export DEBIAN_FRONTEND=noninteractive
case "${PHP_BRANCH:-}" in 83|84|85) ;; *) echo "PHP_BRANCH must be 83, 84 or 85" >&2; exit 1 ;; esac
apt-get update -qq && apt-get install -y -qq curl ca-certificates >/dev/null
VERSION=$PHP_BRANCH
curl -fsSL https://pkg.henderkes.com/api/packages/${VERSION}/debian/repository.key -o /etc/apt/keyrings/static-php${VERSION}.asc
echo "deb [signed-by=/etc/apt/keyrings/static-php${VERSION}.asc] https://pkg.henderkes.com/api/packages/${VERSION}/debian php-zts main" > /etc/apt/sources.list.d/static-php${VERSION}.list
apt-get update -qq
apt-get install -y -qq frankenphp php-zts-cli php-zts-mysqli php-zts-mysqlnd php-zts-curl php-zts-gd php-zts-imagick php-zts-intl php-zts-zip php-zts-bcmath php-zts-fileinfo php-zts-redis php-zts-soap php-zts-igbinary mariadb-server openssh-server less unzip bzip2 logrotate >/dev/null
# Hardening (2026-11-10): Debian security updates install themselves; the base
# image's sudo goes (root is reached with incus exec only). host apply keeps the
# sshd and resolved drop-ins.
apt-get install -y -qq unattended-upgrades msmtp-mta >/dev/null
printf "%s\n" 'APT::Periodic::Update-Package-Lists "1";' 'APT::Periodic::Unattended-Upgrade "1";' > /etc/apt/apt.conf.d/20auto-upgrades
SUDO_FORCE_REMOVE=yes apt-get purge -y -qq sudo >/dev/null 2>&1 || true
# Site user: owns the WordPress files, runs PHP, is the SSH login.
id site >/dev/null 2>&1 || useradd --uid 1000 --create-home --home-dir /srv/site --shell /bin/bash site
mkdir -p /srv/site/public /srv/site/.ssh && chmod 700 /srv/site/.ssh && chown -R site:site /srv/site

# FrankenPHP runs as the site user, so uploads and plugin installs own their files.
mkdir -p /etc/systemd/system/frankenphp.service.d
cat > /etc/systemd/system/frankenphp.service.d/site.conf <<EOF
[Service]
User=site
Group=site
Restart=always
EOF
chown -R site:site /var/lib/frankenphp

cat > /etc/frankenphp/Caddyfile <<"EOF"
# CaptainCore site container. TLS ends at the node edge; this serves plain
# HTTP on :80 to the bridge only. Workers: 2, bursting to 4.
{
	frankenphp {
		num_threads 2
		max_threads 4
		max_wait_time 15s
	}
	auto_https off
	servers {
		trusted_proxies static 10.77.0.1/32
	}
}

:80 {
	root /srv/site/public
	encode zstd gzip
	php_server
}
EOF

cat > /etc/php-zts/conf.d/99-captaincore.ini <<"EOF"
memory_limit = 256M
upload_max_filesize = 128M
post_max_size = 128M
max_execution_time = 120
max_input_vars = 5000
opcache.memory_consumption = 128
opcache.interned_strings_buffer = 16
opcache.max_accelerated_files = 20000
opcache.revalidate_freq = 2
EOF

cat > /etc/mysql/mariadb.conf.d/99-captaincore.cnf <<"EOF"
[mysqld]
bind-address = 127.0.0.1
skip-name-resolve
performance_schema = OFF
max_connections = 40
innodb_buffer_pool_size = 128M
innodb_log_file_size = 64M
EOF

# SSH: keys only, the site user is the login.
cat > /etc/ssh/sshd_config.d/captaincore.conf <<"EOF"
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
AllowUsers site
EOF

# WP-CLI
curl -fsSL -o /usr/local/bin/wp https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar && chmod +x /usr/local/bin/wp

# wp-cron from a systemd timer instead of visitor requests.
cat > /etc/systemd/system/wp-cron.service <<"EOF"
[Unit]
Description=WordPress cron
ConditionPathExists=/srv/site/public/wp-config.php
After=mariadb.service
[Service]
Type=oneshot
User=site
ExecStart=/usr/local/bin/wp --path=/srv/site/public cron event run --due-now --quiet
EOF
cat > /etc/systemd/system/wp-cron.timer <<"EOF"
[Unit]
Description=WordPress cron every minute
[Timer]
OnCalendar=*-*-* *:*:00
AccuracySec=5s
[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable wp-cron.timer frankenphp mariadb ssh >/dev/null 2>&1
frankenphp validate --config /etc/frankenphp/Caddyfile 2>&1 | tail -1
apt-get clean
# Everything a site needs must be in the image: a container rebuilt on it
# (a PHP switch, a refreshed build) keeps nothing from its old system disk.
for need in /usr/bin/frankenphp /usr/sbin/sendmail /usr/local/bin/wp /usr/sbin/sshd /usr/sbin/mariadbd /usr/sbin/logrotate; do
	[ -x "$need" ] || { echo "image is missing $need" >&2; exit 1; }
done
echo IMAGE-SETUP-OK
