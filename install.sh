#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# Redoubt Installer
# Self-hosted team communication platform
#
# Supported: Ubuntu 22.04+, Debian 12+
# Usage:     curl -fsSL https://raw.githubusercontent.com/redoubtapp/redoubt-api/main/install.sh | bash
# =============================================================================

# --- Constants ----------------------------------------------------------------

INSTALL_DIR="/opt/redoubt"
REPO_URL="https://raw.githubusercontent.com/redoubtapp/redoubt-api/main"
MIN_DOCKER_VERSION="24"

# --- Color output helpers ----------------------------------------------------

error()   { echo -e "\033[0;31m✗ ERROR: $1\033[0m"; }
warn()    { echo -e "\033[0;33m⚠ WARNING: $1\033[0m"; }
info()    { echo -e "\033[0;36m→ $1\033[0m"; }
success() { echo -e "\033[0;32m✓ $1\033[0m"; }
header()  { echo -e "\n\033[1;35m$1\033[0m"; }

# --- Secret generation helpers -----------------------------------------------

generate_secret() {
    local length=${1:-32}
    openssl rand -base64 "$length" | tr -d '\n'
}

generate_alphanumeric() {
    local length=${1:-32}
    tr -dc 'A-Za-z0-9' < /dev/urandom | head -c "$length"
}

# --- System checks -----------------------------------------------------------

check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "This script must be run as root (use sudo)"
        exit 1
    fi
}

check_os() {
    if [[ ! -f /etc/os-release ]]; then
        error "Cannot detect OS. /etc/os-release not found."
        exit 1
    fi

    # shellcheck source=/dev/null
    source /etc/os-release

    case "$ID" in
        ubuntu)
            local major_version
            major_version=$(echo "$VERSION_ID" | cut -d. -f1)
            if [[ "$major_version" -lt 22 ]]; then
                error "Ubuntu 22.04 or later is required (found $VERSION_ID)"
                exit 1
            fi
            success "OS detected: Ubuntu $VERSION_ID"
            ;;
        debian)
            local major_version
            major_version=$(echo "$VERSION_ID" | cut -d. -f1)
            if [[ "$major_version" -lt 12 ]]; then
                error "Debian 12 or later is required (found $VERSION_ID)"
                exit 1
            fi
            success "OS detected: Debian $VERSION_ID"
            ;;
        *)
            error "Unsupported OS: $ID. Only Ubuntu 22.04+ and Debian 12+ are supported."
            exit 1
            ;;
    esac
}

check_arch() {
    local arch
    arch=$(dpkg --print-architecture 2>/dev/null || uname -m)

    case "$arch" in
        amd64|x86_64)
            success "Architecture: amd64"
            ;;
        arm64|aarch64)
            success "Architecture: arm64"
            ;;
        *)
            error "Unsupported architecture: $arch. Only amd64 and arm64 are supported."
            exit 1
            ;;
    esac
}

check_resources() {
    local vcpus
    vcpus=$(nproc)
    if [[ "$vcpus" -lt 2 ]]; then
        warn "Only $vcpus vCPU(s) detected. Minimum recommended is 2."
    else
        success "CPUs: $vcpus"
    fi

    local total_mem_kb
    total_mem_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    local total_mem_gb=$(( total_mem_kb / 1024 / 1024 ))
    if [[ "$total_mem_gb" -lt 4 ]]; then
        warn "Only ${total_mem_gb}GB RAM detected. Minimum recommended is 4GB."
    else
        success "Memory: ${total_mem_gb}GB"
    fi
}

check_ports() {
    # Ensure ss is available (it should be on any modern Debian/Ubuntu)
    for port in 80 443; do
        if ss -tlnp | grep -q ":${port} "; then
            error "Port $port is already in use:"
            ss -tlnp | grep ":${port} " || true
            error "Free port $port before running this installer."
            exit 1
        fi
    done
    success "Ports 80 and 443 are available"
}

# --- Docker ------------------------------------------------------------------

install_docker() {
    if command -v docker &>/dev/null; then
        local docker_version
        docker_version=$(docker version --format '{{.Server.Version}}' 2>/dev/null | cut -d. -f1)
        if [[ "$docker_version" -ge "$MIN_DOCKER_VERSION" ]]; then
            success "Docker $docker_version already installed"
            return
        fi
    fi

    info "Installing Docker..."
    curl -fsSL https://get.docker.com | bash
    success "Docker installed"
}

# --- System dependencies -----------------------------------------------------

install_dependencies() {
    local missing=()
    command -v curl  &>/dev/null || missing+=(curl)
    command -v dig   &>/dev/null || missing+=(dnsutils)
    command -v ufw   &>/dev/null || missing+=(ufw)

    if [[ ${#missing[@]} -gt 0 ]]; then
        info "Installing missing packages: ${missing[*]}"
        apt-get update -qq
        apt-get install -y "${missing[@]}"
    fi
    success "System dependencies installed"
}

# --- Domain & DNS ------------------------------------------------------------

prompt_domain() {
    read -rp "Enter your domain name (e.g., chat.example.com): " DOMAIN
    if [[ -z "$DOMAIN" ]]; then
        error "Domain name cannot be empty."
        exit 1
    fi
}

verify_dns() {
    local domain="$1"
    info "Verifying DNS for $domain..."

    local server_ip
    server_ip=$(curl -s --max-time 5 ifconfig.me)

    local domain_ip
    domain_ip=$(dig +short "$domain" A | head -1)

    if [[ -z "$domain_ip" ]]; then
        error "Could not resolve '$domain'. Make sure the DNS A record is configured."
        error "Point '$domain' to this server's IP: $server_ip"
        exit 1
    fi

    if [[ "$domain_ip" != "$server_ip" ]]; then
        error "DNS mismatch: '$domain' resolves to $domain_ip, but this server's IP is $server_ip"
        error "Update the DNS A record for '$domain' to point to $server_ip"
        error "DNS changes can take up to 48 hours to propagate, but usually take 5-15 minutes."
        exit 1
    fi

    success "DNS verified: $domain -> $server_ip"
}

# --- Email -------------------------------------------------------------------

prompt_email() {
    read -rp "Enter email for TLS certificates (Let's Encrypt): " ACME_EMAIL
    if [[ -z "$ACME_EMAIL" ]]; then
        error "Email cannot be empty. Let's Encrypt requires an email for certificate notifications."
        exit 1
    fi
}

# --- Resend (email delivery) ------------------------------------------------

prompt_resend() {
    echo
    info "Resend is used for transactional emails (password resets, email verification)."
    info "Get an API key at https://resend.com (free tier: 3,000 emails/month)"
    echo
    read -rp "Resend API key (leave blank to skip, can configure later): " RESEND_API_KEY
    if [[ -n "$RESEND_API_KEY" ]]; then
        success "Resend API key set"
    else
        warn "Skipped. Email features (password reset, verification) will not work until configured."
        info "Set RESEND_API_KEY in $INSTALL_DIR/.env later."
    fi
}

# --- S3 storage --------------------------------------------------------------

prompt_s3() {
    echo
    info "S3-compatible storage is used for file uploads (avatars, attachments)."
    info "Any S3-compatible provider works (AWS S3, Backblaze B2, MinIO, etc.)"
    echo
    read -rp "S3 access key (leave blank to skip, can configure later): " S3_ACCESS_KEY
    if [[ -n "$S3_ACCESS_KEY" ]]; then
        read -rp "S3 secret key: " S3_SECRET_KEY
        if [[ -z "$S3_SECRET_KEY" ]]; then
            error "S3 secret key cannot be empty if access key is provided."
            exit 1
        fi
        read -rp "S3 bucket name: " S3_BUCKET
        if [[ -z "$S3_BUCKET" ]]; then
            error "S3 bucket name cannot be empty."
            exit 1
        fi
        read -rp "S3 region (e.g., us-east-1): " S3_REGION
        S3_REGION=${S3_REGION:-us-east-1}
        read -rp "S3 endpoint URL (leave blank for AWS S3): " S3_ENDPOINT
        success "S3 storage configured"
    else
        warn "Skipped. File uploads (avatars, attachments) will not work until configured."
        info "Set S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET in $INSTALL_DIR/.env later."
        S3_SECRET_KEY=""
        S3_BUCKET=""
        S3_REGION=""
        S3_ENDPOINT=""
    fi
}

# --- Secrets -----------------------------------------------------------------

generate_all_secrets() {
    POSTGRES_PASSWORD=$(generate_alphanumeric 32)
    JWT_SECRET=$(generate_secret 64)
    STORAGE_MASTER_KEY=$(generate_secret 32)
    LIVEKIT_API_KEY="API$(generate_alphanumeric 12)"
    LIVEKIT_API_SECRET=$(generate_secret 32)
    ADMIN_SESSION_SECRET=$(generate_secret 32)
    success "All secrets generated"
}

# --- Directory setup ---------------------------------------------------------

create_directories() {
    mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/config"
    success "Created $INSTALL_DIR"
}

# --- File downloads ----------------------------------------------------------

download_files() {
    info "Downloading configuration files..."

    curl -fsSL "$REPO_URL/docker-compose.prod.yml" -o "$INSTALL_DIR/docker-compose.yml"
    curl -fsSL "$REPO_URL/Caddyfile"               -o "$INSTALL_DIR/Caddyfile"
    curl -fsSL "$REPO_URL/config/config.yaml"       -o "$INSTALL_DIR/config/config.yaml"
    curl -fsSL "$REPO_URL/config/livekit.yaml"      -o "$INSTALL_DIR/config/livekit.yaml"

    success "Configuration files downloaded"
}

# --- .env file ---------------------------------------------------------------

write_env_file() {
    if [[ -f "$INSTALL_DIR/.env" ]]; then
        info ".env already exists -- skipping (secrets preserved)"
        return
    fi

    cat > "$INSTALL_DIR/.env" <<ENVEOF
# Generated by Redoubt installer on $(date -u +"%Y-%m-%d %H:%M:%S UTC")
# Do not edit unless you know what you're doing.

# Domain
DOMAIN=${DOMAIN}

# Email (for Let's Encrypt notifications)
ACME_EMAIL=${ACME_EMAIL}

# Database
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}

# Authentication
JWT_SECRET=${JWT_SECRET}

# Storage encryption
STORAGE_MASTER_KEY=${STORAGE_MASTER_KEY}

# LiveKit
LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}

# Admin panel
ADMIN_SESSION_SECRET=${ADMIN_SESSION_SECRET}

# Email (Resend — for password resets, email verification)
RESEND_API_KEY=${RESEND_API_KEY:-}

# S3 storage (avatars, file uploads)
S3_ACCESS_KEY=${S3_ACCESS_KEY:-}
S3_SECRET_KEY=${S3_SECRET_KEY:-}
S3_BUCKET=${S3_BUCKET:-}
S3_REGION=${S3_REGION:-}
S3_ENDPOINT=${S3_ENDPOINT:-}
ENVEOF

    chmod 600 "$INSTALL_DIR/.env"
    success "Environment file written to $INSTALL_DIR/.env"
}

# --- Firewall ----------------------------------------------------------------

configure_firewall() {
    if ! command -v ufw &>/dev/null; then
        apt-get install -y ufw
    fi

    ufw default deny incoming
    ufw default allow outgoing
    ufw allow 22/tcp  comment "SSH"
    ufw allow 80/tcp  comment "HTTP"
    ufw allow 443/tcp comment "HTTPS"
    ufw allow 7881/tcp comment "LiveKit RTC TCP"
    ufw allow 7882/udp comment "LiveKit TURN UDP"
    ufw allow 60000:60100/udp comment "WebRTC media"
    ufw --force enable

    success "Firewall configured"
}

# --- Health check ------------------------------------------------------------

wait_for_health() {
    local timeout=120
    local elapsed=0
    info "Waiting for services to start (timeout: ${timeout}s)..."

    cd "$INSTALL_DIR"
    while [[ $elapsed -lt $timeout ]]; do
        if curl -sfk https://localhost/health > /dev/null 2>&1; then
            echo
            success "All services are healthy"
            return 0
        fi
        sleep 5
        elapsed=$((elapsed + 5))
        printf "."
    done

    echo
    error "Services did not become healthy within ${timeout}s"
    error "Check logs with: cd $INSTALL_DIR && docker compose logs"
    exit 1
}

# --- Bootstrap code ----------------------------------------------------------

extract_bootstrap_code() {
    BOOTSTRAP_CODE=$(docker compose -f "$INSTALL_DIR/docker-compose.yml" logs redoubt-api 2>/dev/null \
        | grep -o '"code":"[^"]*"' | head -1 | cut -d'"' -f4) || true

    if [[ -z "${BOOTSTRAP_CODE:-}" ]]; then
        BOOTSTRAP_CODE="(check logs: cd $INSTALL_DIR && docker compose logs redoubt-api | grep code)"
    fi
}

# --- Upgrade flow ------------------------------------------------------------

upgrade_flow() {
    echo
    read -rp "An existing Redoubt installation was found. Do you want to upgrade? (y/n): " answer
    if [[ "$answer" != "y" && "$answer" != "Y" ]]; then
        info "Upgrade cancelled."
        exit 0
    fi

    header "Upgrading Redoubt"

    cd "$INSTALL_DIR"

    info "Downloading latest configuration files..."
    curl -fsSL "$REPO_URL/docker-compose.prod.yml" -o "$INSTALL_DIR/docker-compose.yml"
    curl -fsSL "$REPO_URL/Caddyfile"               -o "$INSTALL_DIR/Caddyfile"
    curl -fsSL "$REPO_URL/config/config.yaml"       -o "$INSTALL_DIR/config/config.yaml"
    curl -fsSL "$REPO_URL/config/livekit.yaml"      -o "$INSTALL_DIR/config/livekit.yaml"
    success "Configuration files updated (.env preserved)"

    info "Pulling latest images..."
    docker compose pull

    info "Restarting services..."
    docker compose up -d

    wait_for_health

    echo
    success "Upgrade complete!"
}

# --- Success banner ----------------------------------------------------------

print_success_banner() {
    echo
    echo "=================================================================="
    echo ""
    echo "  Redoubt is running!"
    echo ""
    echo "=================================================================="
    echo ""
    echo "  Your instance is live at:"
    echo "    https://$DOMAIN"
    echo ""
    echo "  Bootstrap invite code:"
    echo "    $BOOTSTRAP_CODE"
    echo ""
    echo "  Use this code to register the first user."
    echo "  The first user becomes the instance administrator."
    echo ""
    echo "  Admin panel (via SSH tunnel):"
    echo "    ssh -L 9091:localhost:9091 user@your-server"
    echo "    Then open: http://localhost:9091"
    echo ""
    echo "  Useful commands:"
    echo "    cd /opt/redoubt"
    echo "    docker compose logs -f          # View logs"
    echo "    docker compose restart          # Restart services"
    echo "    docker compose pull && \\"
    echo "      docker compose up -d          # Upgrade"
    echo ""
    echo "  Config: /opt/redoubt/.env"
    echo "  Data:   Docker volumes (postgres_data, redis_data, etc.)"
    echo ""
    echo "  Documentation:"
    echo "    https://github.com/redoubtapp/redoubt-api/docs"
    echo ""
    echo "=================================================================="
    echo
}

# --- Main --------------------------------------------------------------------

main() {
    header "Redoubt Installer"
    echo "Self-hosted team communication platform"
    echo

    # System checks
    check_root
    check_os
    check_arch
    check_resources
    check_ports

    # Check for existing installation (upgrade path)
    if [[ -d "$INSTALL_DIR" ]]; then
        upgrade_flow
        exit 0
    fi

    # Install prerequisites
    install_docker
    install_dependencies

    # Interactive prompts
    header "Configuration"
    prompt_domain
    verify_dns "$DOMAIN"
    prompt_email
    prompt_resend
    prompt_s3

    # Generate secrets
    header "Generating Secrets"
    generate_all_secrets

    # Setup
    header "Setting Up Redoubt"
    create_directories
    download_files
    write_env_file

    # Firewall
    header "Configuring Firewall"
    configure_firewall

    # Start services
    header "Starting Services"
    cd "$INSTALL_DIR"
    docker compose pull
    docker compose up -d

    # Wait for health
    wait_for_health

    # Get bootstrap code
    extract_bootstrap_code

    # Done!
    print_success_banner
}

main "$@"
