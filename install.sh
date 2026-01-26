#!/bin/bash
#
# Elchi CoreDNS Plugin Installer
#
# Supported distributions:
#   - Ubuntu 18.04+
#   - Debian 10+
#   - Oracle Linux 7+
#   - RHEL/CentOS 7+
#   - Fedora 30+
#   - Amazon Linux 2
#
# Usage:
#   curl -fsSL https://github.com/cloudnativeworks/elchi-gslb/releases/download/v${VERSION}/install.sh | sudo bash
#   curl -fsSL https://github.com/cloudnativeworks/elchi-gslb/releases/download/v${VERSION}/install.sh | sudo bash -s -- --zone gslb.example.com --endpoint http://controller:8080 --secret mysecret
#
# Options:
#   --zone ZONE           DNS zone to serve (default: gslb.elchi)
#   --endpoint URL        Elchi controller endpoint (default: http://localhost:8080)
#   --secret SECRET       Authentication secret (default: change-me-in-production)
#   --port PORT           DNS port to listen on (default: 53)
#   --webhook-port PORT   Webhook API port (default: 8053)
#   --version VERSION     Specific version to install (default: version matching this script)
#   --disable-resolved    Disable systemd-resolved (required for port 53)
#   --uninstall           Uninstall elchi-coredns
#   --help                Show this help message
#

set -e

# Script version - this should match the release version
SCRIPT_VERSION="0.1.5"

# Default values
ZONE="gslb.elchi"
ENDPOINT="http://localhost:8080"
SECRET="change-me-in-production"
DNS_PORT="53"
WEBHOOK_PORT="8053"
VERSION="${SCRIPT_VERSION}"
UNINSTALL=false
DISABLE_RESOLVED=false

# Installation paths
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/coredns"
SERVICE_NAME="coredns-elchi"
BINARY_NAME="coredns-elchi"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $1"
}

# Print banner
print_banner() {
    echo ""
    echo -e "${CYAN}"
    echo "  _____ _      ____ _   _ ___    ____ ____  _     ____  "
    echo " | ____| |    / ___| | | |_ _|  / ___/ ___|| |   | __ ) "
    echo " |  _| | |   | |   | |_| || |  | |  _\___ \| |   |  _ \ "
    echo " | |___| |___| |___|  _  || |  | |_| |___) | |___| |_) |"
    echo " |_____|_____|\____|_| |_|___|  \____|____/|_____|____/ "
    echo -e "${NC}"
    echo " CoreDNS Plugin Installer v${SCRIPT_VERSION}"
    echo ""
}

# Show help
show_help() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --zone ZONE           DNS zone to serve (default: ${ZONE})"
    echo "  --endpoint URL        Elchi controller endpoint (default: ${ENDPOINT})"
    echo "  --secret SECRET       Authentication secret (default: change-me-in-production)"
    echo "  --port PORT           DNS port to listen on (default: ${DNS_PORT})"
    echo "  --webhook-port PORT   Webhook API port (default: ${WEBHOOK_PORT})"
    echo "  --version VERSION     Specific version to install (default: ${VERSION})"
    echo "  --disable-resolved    Disable systemd-resolved to free port 53"
    echo "  --uninstall           Uninstall elchi-coredns"
    echo "  --help                Show this help message"
    echo ""
    echo "Examples:"
    echo "  # Install with defaults (configure manually later)"
    echo "  sudo $0"
    echo ""
    echo "  # Install with custom configuration"
    echo "  sudo $0 --zone lb.example.com --endpoint http://10.0.0.1:8080 --secret myS3cr3t!"
    echo ""
    echo "  # Install and disable systemd-resolved"
    echo "  sudo $0 --disable-resolved"
    echo ""
    echo "  # Install specific version"
    echo "  sudo $0 --version 0.1.5"
    echo ""
    echo "  # Uninstall"
    echo "  sudo $0 --uninstall"
    echo ""
    echo "Supported distributions:"
    echo "  - Ubuntu 18.04+"
    echo "  - Debian 10+"
    echo "  - Oracle Linux 7+"
    echo "  - RHEL/CentOS 7+"
    echo "  - Fedora 30+"
    echo "  - Amazon Linux 2"
    echo ""
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --zone)
                ZONE="$2"
                shift 2
                ;;
            --endpoint)
                ENDPOINT="$2"
                shift 2
                ;;
            --secret)
                SECRET="$2"
                shift 2
                ;;
            --port)
                DNS_PORT="$2"
                shift 2
                ;;
            --webhook-port)
                WEBHOOK_PORT="$2"
                shift 2
                ;;
            --version)
                VERSION="$2"
                shift 2
                ;;
            --disable-resolved)
                DISABLE_RESOLVED=true
                shift
                ;;
            --uninstall)
                UNINSTALL=true
                shift
                ;;
            --help|-h)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "${ARCH}" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            log_error "Unsupported architecture: ${ARCH}"
            exit 1
            ;;
    esac

    case "${OS}" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            log_error "Unsupported OS: ${OS}"
            exit 1
            ;;
    esac

    # Detect Linux distribution
    DISTRO="unknown"
    DISTRO_VERSION=""
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        DISTRO="${ID}"
        DISTRO_VERSION="${VERSION_ID}"
    elif [[ -f /etc/redhat-release ]]; then
        DISTRO="rhel"
    elif [[ -f /etc/debian_version ]]; then
        DISTRO="debian"
    fi

    log_info "Detected platform: ${OS}/${ARCH}"
    if [[ "${OS}" == "linux" ]]; then
        log_info "Distribution: ${DISTRO} ${DISTRO_VERSION}"
    fi
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

# Check dependencies
check_dependencies() {
    local missing=()

    if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
        missing+=("curl or wget")
    fi

    if ! command -v systemctl &> /dev/null && [[ "${OS}" == "linux" ]]; then
        log_warn "systemctl not found. Service management will be skipped."
        log_warn "You will need to manage the service manually."
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing dependencies: ${missing[*]}"
        log_error "Please install them and try again."

        # Suggest installation commands based on distro
        case "${DISTRO}" in
            ubuntu|debian)
                log_info "Try: apt-get update && apt-get install -y curl"
                ;;
            centos|rhel|fedora|ol|oracle)
                log_info "Try: yum install -y curl"
                ;;
            amzn)
                log_info "Try: yum install -y curl"
                ;;
        esac
        exit 1
    fi
}

# Disable systemd-resolved
disable_systemd_resolved() {
    if ! command -v systemctl &> /dev/null; then
        return
    fi

    if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
        log_info "Disabling systemd-resolved..."

        systemctl stop systemd-resolved
        systemctl disable systemd-resolved

        # Remove the symlink to systemd-resolved's stub resolver
        if [[ -L /etc/resolv.conf ]]; then
            rm -f /etc/resolv.conf
            # Create a basic resolv.conf
            cat > /etc/resolv.conf << EOF
# Generated by elchi-coredns installer
nameserver 8.8.8.8
nameserver 8.8.4.4
EOF
        fi

        log_success "systemd-resolved disabled"
    else
        log_info "systemd-resolved is not active"
    fi
}

# Uninstall function
uninstall() {
    log_step "Uninstalling ${SERVICE_NAME}..."

    # Stop and disable service
    if command -v systemctl &> /dev/null; then
        if systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
            log_info "Stopping ${SERVICE_NAME} service..."
            systemctl stop ${SERVICE_NAME}
        fi
        if systemctl is-enabled --quiet ${SERVICE_NAME} 2>/dev/null; then
            log_info "Disabling ${SERVICE_NAME} service..."
            systemctl disable ${SERVICE_NAME}
        fi
        if [[ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]]; then
            log_info "Removing systemd service file..."
            rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
            systemctl daemon-reload
        fi
    fi

    # Remove binary
    if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        log_info "Removing binary..."
        rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    # Ask about config removal
    if [[ -d "${CONFIG_DIR}" ]]; then
        echo ""
        log_warn "Configuration directory ${CONFIG_DIR} still exists."
        log_warn "Remove it manually if you want to delete all configuration:"
        log_warn "  sudo rm -rf ${CONFIG_DIR}"
    fi

    echo ""
    log_success "Uninstallation complete!"
}

# Download binary
download_binary() {
    local url="https://github.com/cloudnativeworks/elchi-gslb/releases/download/v${VERSION}/coredns-elchi-${OS}-${ARCH}-v${VERSION}"
    local tmp_file="/tmp/${BINARY_NAME}"

    log_step "Downloading elchi-coredns v${VERSION}..."
    log_info "URL: ${url}"

    # Try curl first, then wget
    if command -v curl &> /dev/null; then
        if ! curl -fsSL -o "${tmp_file}" "${url}"; then
            log_error "Failed to download binary from ${url}"
            log_error "Please check if version v${VERSION} exists."
            exit 1
        fi
    elif command -v wget &> /dev/null; then
        if ! wget -q -O "${tmp_file}" "${url}"; then
            log_error "Failed to download binary from ${url}"
            log_error "Please check if version v${VERSION} exists."
            exit 1
        fi
    fi

    chmod +x "${tmp_file}"
    mv "${tmp_file}" "${INSTALL_DIR}/${BINARY_NAME}"

    log_success "Binary installed to ${INSTALL_DIR}/${BINARY_NAME}"
}

# Create configuration directory and files
create_config() {
    log_step "Creating configuration..."

    # Create config directory
    mkdir -p "${CONFIG_DIR}"

    # Create Corefile
    cat > "${CONFIG_DIR}/Corefile" << EOF
# Elchi GSLB CoreDNS Configuration
# Generated by install.sh v${SCRIPT_VERSION}
# Distribution: ${DISTRO} ${DISTRO_VERSION}
#
# Edit this file to customize your DNS configuration.
# After making changes, restart the service:
#   sudo systemctl restart ${SERVICE_NAME}

# Main zone configuration
${ZONE}:${DNS_PORT} {
    elchi {
        # Elchi controller endpoint
        endpoint ${ENDPOINT}

        # Authentication secret (CHANGE THIS IN PRODUCTION!)
        secret ${SECRET}

        # Default TTL for records (seconds)
        ttl 300

        # How often to sync with controller
        sync_interval 5m

        # HTTP timeout for controller requests
        timeout 10s

        # Webhook server for instant updates
        webhook :${WEBHOOK_PORT}
    }

    # Enable logging (comment out for less verbose output)
    log

    # Enable error logging
    errors

    # Enable Prometheus metrics
    prometheus :9253

    # Health check endpoint (for load balancers)
    health :8080

    # Ready check endpoint (for Kubernetes)
    ready :8181
}

# Catch-all for other queries (forward to upstream DNS)
.:${DNS_PORT} {
    forward . 8.8.8.8 8.8.4.4
    log
    errors
    cache 30
}
EOF

    log_success "Configuration created at ${CONFIG_DIR}/Corefile"

    # Show configuration summary
    echo ""
    log_info "Configuration Summary:"
    echo "  Zone:          ${ZONE}"
    echo "  DNS Port:      ${DNS_PORT}"
    echo "  Endpoint:      ${ENDPOINT}"
    echo "  Webhook Port:  ${WEBHOOK_PORT}"
    echo "  Config File:   ${CONFIG_DIR}/Corefile"
    echo ""
}

# Create systemd service
create_systemd_service() {
    if ! command -v systemctl &> /dev/null; then
        log_warn "systemctl not available. Skipping service creation."
        return
    fi

    log_step "Creating systemd service..."

    cat > "/etc/systemd/system/${SERVICE_NAME}.service" << EOF
[Unit]
Description=Elchi GSLB CoreDNS Server
Documentation=https://github.com/cloudnativeworks/elchi-gslb
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -conf ${CONFIG_DIR}/Corefile
ExecReload=/bin/kill -SIGUSR1 \$MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576
LimitNPROC=512

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
ReadWritePaths=${CONFIG_DIR}

# Allow binding to privileged ports (53)
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    log_success "Systemd service created"
}

# Check if port is in use
check_port() {
    local port=$1

    if command -v ss &> /dev/null; then
        ss -tuln 2>/dev/null | grep -q ":${port} "
    elif command -v netstat &> /dev/null; then
        netstat -tuln 2>/dev/null | grep -q ":${port} "
    else
        return 1
    fi
}

# Enable and start service
start_service() {
    if ! command -v systemctl &> /dev/null; then
        log_warn "systemctl not available. Please start the service manually:"
        echo ""
        echo "  ${INSTALL_DIR}/${BINARY_NAME} -conf ${CONFIG_DIR}/Corefile"
        echo ""
        return
    fi

    log_step "Enabling and starting ${SERVICE_NAME} service..."

    systemctl enable ${SERVICE_NAME} 2>/dev/null

    # Check if port 53 is in use
    if check_port "${DNS_PORT}"; then
        echo ""
        log_warn "Port ${DNS_PORT} is already in use!"
        log_warn ""
        log_warn "This is usually caused by systemd-resolved."
        log_warn "You have two options:"
        echo ""
        echo "  Option 1: Disable systemd-resolved (recommended for DNS servers)"
        echo "    sudo systemctl stop systemd-resolved"
        echo "    sudo systemctl disable systemd-resolved"
        echo "    sudo rm /etc/resolv.conf"
        echo "    echo 'nameserver 8.8.8.8' | sudo tee /etc/resolv.conf"
        echo "    sudo systemctl start ${SERVICE_NAME}"
        echo ""
        echo "  Option 2: Use a different port"
        echo "    Edit ${CONFIG_DIR}/Corefile and change the port"
        echo "    sudo systemctl start ${SERVICE_NAME}"
        echo ""

        if [[ "${DISABLE_RESOLVED}" == true ]]; then
            log_info "Disabling systemd-resolved as requested..."
            disable_systemd_resolved
            systemctl start ${SERVICE_NAME}
        else
            log_warn "Service NOT started. Please resolve the port conflict first."
            log_warn "Or re-run with --disable-resolved flag."
            return
        fi
    else
        systemctl start ${SERVICE_NAME}
    fi

    # Wait a moment and check status
    sleep 2
    if systemctl is-active --quiet ${SERVICE_NAME}; then
        log_success "Service started successfully!"
    else
        log_error "Service failed to start. Check logs with:"
        echo "  sudo journalctl -u ${SERVICE_NAME} -f"
    fi
}

# Print post-installation instructions
print_instructions() {
    echo ""
    echo -e "${GREEN}=============================================="
    echo " Installation Complete!"
    echo "==============================================${NC}"
    echo ""
    echo "Quick Commands:"
    echo "  Check service status:  sudo systemctl status ${SERVICE_NAME}"
    echo "  View logs:             sudo journalctl -u ${SERVICE_NAME} -f"
    echo "  Restart service:       sudo systemctl restart ${SERVICE_NAME}"
    echo "  Stop service:          sudo systemctl stop ${SERVICE_NAME}"
    echo ""
    echo "Configuration:"
    echo "  Edit config:           sudo nano ${CONFIG_DIR}/Corefile"
    echo ""
    echo "Test DNS:"
    echo "  dig @localhost ${ZONE} A"
    echo "  dig @localhost test.${ZONE} A"
    echo ""
    echo "Webhook Endpoints (port ${WEBHOOK_PORT}):"
    echo "  Health check:          curl http://localhost:${WEBHOOK_PORT}/health"
    echo "  List records:          curl -H 'X-Elchi-Secret: YOUR_SECRET' http://localhost:${WEBHOOK_PORT}/records"
    echo ""
    echo "Prometheus Metrics:"
    echo "  curl http://localhost:9253/metrics"
    echo ""

    if [[ "${SECRET}" == "change-me-in-production" ]]; then
        echo -e "${YELLOW}=============================================="
        echo " ACTION REQUIRED"
        echo "==============================================${NC}"
        echo ""
        log_warn "You are using the default secret!"
        log_warn "Please update the secret in ${CONFIG_DIR}/Corefile"
        log_warn "and restart the service:"
        echo ""
        echo "  1. Edit the config:  sudo nano ${CONFIG_DIR}/Corefile"
        echo "  2. Change 'secret change-me-in-production' to your secret"
        echo "  3. Restart service:  sudo systemctl restart ${SERVICE_NAME}"
        echo ""
    fi
}

# Main installation flow
main() {
    print_banner
    parse_args "$@"

    if [[ "${UNINSTALL}" == true ]]; then
        check_root
        uninstall
        exit 0
    fi

    check_root
    detect_platform
    check_dependencies

    # Handle systemd-resolved early if requested
    if [[ "${DISABLE_RESOLVED}" == true ]]; then
        disable_systemd_resolved
    fi

    # Check for existing installation
    if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        log_warn "Existing installation found."
        log_info "Upgrading to v${VERSION}..."

        # Stop service if running
        if command -v systemctl &> /dev/null && systemctl is-active --quiet ${SERVICE_NAME} 2>/dev/null; then
            log_info "Stopping existing service..."
            systemctl stop ${SERVICE_NAME}
        fi
    fi

    download_binary

    # Only create config if it doesn't exist
    if [[ ! -f "${CONFIG_DIR}/Corefile" ]]; then
        create_config
    else
        log_info "Existing configuration found at ${CONFIG_DIR}/Corefile"
        log_info "Keeping existing configuration."
        log_info "To update configuration, edit: ${CONFIG_DIR}/Corefile"
    fi

    create_systemd_service
    start_service
    print_instructions
}

# Run main function
main "$@"
