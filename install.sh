#!/bin/bash
#
# Elchi CoreDNS Plugin Installer
#
# Usage:
#   curl -fsSL https://github.com/cloudnativeworks/elchi-gslb/releases/download/v${VERSION}/install.sh | bash
#   curl -fsSL https://github.com/cloudnativeworks/elchi-gslb/releases/download/v${VERSION}/install.sh | bash -s -- --zone gslb.example.com --endpoint http://controller:8080 --secret mysecret
#
# Options:
#   --zone ZONE           DNS zone to serve (default: gslb.elchi)
#   --endpoint URL        Elchi controller endpoint (default: http://localhost:8080)
#   --secret SECRET       Authentication secret (default: change-me-in-production)
#   --port PORT           DNS port to listen on (default: 53)
#   --webhook-port PORT   Webhook API port (default: 8053)
#   --version VERSION     Specific version to install (default: version matching this script)
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

# Print banner
print_banner() {
    echo ""
    echo "  _____ _      _     _    ____  ____  _     ____  "
    echo " | ____| |    | |   | |  / ___|/ ___|| |   | __ ) "
    echo " |  _| | |    | |   | | | |  _\___ \| |   |  _ \ "
    echo " | |___| |___ | |___| | | |_| |___) | |___| |_) |"
    echo " |_____|_____|_____|_|  \____|____/|_____|____/ "
    echo ""
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
    echo "  --uninstall           Uninstall elchi-coredns"
    echo "  --help                Show this help message"
    echo ""
    echo "Examples:"
    echo "  # Install with defaults (configure manually later)"
    echo "  $0"
    echo ""
    echo "  # Install with custom configuration"
    echo "  $0 --zone lb.example.com --endpoint http://10.0.0.1:8080 --secret myS3cr3t!"
    echo ""
    echo "  # Install specific version"
    echo "  $0 --version 0.1.5"
    echo ""
    echo "  # Uninstall"
    echo "  $0 --uninstall"
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

    log_info "Detected platform: ${OS}/${ARCH}"
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

    if ! command -v curl &> /dev/null; then
        missing+=("curl")
    fi

    if ! command -v systemctl &> /dev/null && [[ "${OS}" == "linux" ]]; then
        log_warn "systemctl not found. Service management will be skipped."
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing dependencies: ${missing[*]}"
        log_error "Please install them and try again."
        exit 1
    fi
}

# Uninstall function
uninstall() {
    log_info "Uninstalling ${SERVICE_NAME}..."

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
        log_warn "Configuration directory ${CONFIG_DIR} still exists."
        log_warn "Remove it manually if you want to delete all configuration:"
        log_warn "  sudo rm -rf ${CONFIG_DIR}"
    fi

    log_success "Uninstallation complete!"
}

# Download binary
download_binary() {
    local url="https://github.com/cloudnativeworks/elchi-gslb/releases/download/v${VERSION}/coredns-elchi-${OS}-${ARCH}-v${VERSION}"
    local tmp_file="/tmp/${BINARY_NAME}"

    log_info "Downloading elchi-coredns v${VERSION}..."
    log_info "URL: ${url}"

    if ! curl -fsSL -o "${tmp_file}" "${url}"; then
        log_error "Failed to download binary from ${url}"
        log_error "Please check if version v${VERSION} exists."
        exit 1
    fi

    chmod +x "${tmp_file}"
    mv "${tmp_file}" "${INSTALL_DIR}/${BINARY_NAME}"

    log_success "Binary installed to ${INSTALL_DIR}/${BINARY_NAME}"
}

# Create configuration directory and files
create_config() {
    log_info "Creating configuration..."

    # Create config directory
    mkdir -p "${CONFIG_DIR}"

    # Create Corefile
    cat > "${CONFIG_DIR}/Corefile" << EOF
# Elchi GSLB CoreDNS Configuration
# Generated by install.sh v${SCRIPT_VERSION}
#
# Edit this file to customize your DNS configuration.
# After making changes, restart the service:
#   sudo systemctl restart ${SERVICE_NAME}

# Main zone configuration
${ZONE}:${DNS_PORT} {
    elchi {
        endpoint ${ENDPOINT}
        secret ${SECRET}
        ttl 300
        sync_interval 5m
        timeout 10s
        webhook :${WEBHOOK_PORT}
    }

    # Enable logging (comment out for less verbose output)
    log

    # Enable error logging
    errors

    # Enable Prometheus metrics on port 9253
    prometheus :9253

    # Health check endpoint
    health :8080

    # Ready check endpoint
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

    log_info "Creating systemd service..."

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

# Enable and start service
start_service() {
    if ! command -v systemctl &> /dev/null; then
        log_warn "systemctl not available. Please start the service manually:"
        log_warn "  ${INSTALL_DIR}/${BINARY_NAME} -conf ${CONFIG_DIR}/Corefile"
        return
    fi

    log_info "Enabling and starting ${SERVICE_NAME} service..."

    systemctl enable ${SERVICE_NAME}

    # Check if port 53 is in use
    if ss -tuln | grep -q ":${DNS_PORT} "; then
        log_warn "Port ${DNS_PORT} is already in use!"
        log_warn "You may need to stop the existing DNS service first:"
        log_warn "  sudo systemctl stop systemd-resolved"
        log_warn "  sudo systemctl disable systemd-resolved"
        log_warn ""
        log_warn "Then start ${SERVICE_NAME}:"
        log_warn "  sudo systemctl start ${SERVICE_NAME}"
    else
        systemctl start ${SERVICE_NAME}

        # Wait a moment and check status
        sleep 2
        if systemctl is-active --quiet ${SERVICE_NAME}; then
            log_success "Service started successfully!"
        else
            log_error "Service failed to start. Check logs with:"
            log_error "  sudo journalctl -u ${SERVICE_NAME} -f"
        fi
    fi
}

# Print post-installation instructions
print_instructions() {
    echo ""
    echo "=============================================="
    echo " Installation Complete!"
    echo "=============================================="
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
    echo "Webhook Endpoints:"
    echo "  Health check:          curl http://localhost:${WEBHOOK_PORT}/health"
    echo "  List records:          curl -H 'X-Elchi-Secret: ${SECRET}' http://localhost:${WEBHOOK_PORT}/records"
    echo ""
    echo "Prometheus Metrics:"
    echo "  curl http://localhost:9253/metrics"
    echo ""

    if [[ "${SECRET}" == "change-me-in-production" ]]; then
        log_warn "WARNING: You are using the default secret!"
        log_warn "Please update the secret in ${CONFIG_DIR}/Corefile"
        log_warn "and restart the service."
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
    fi

    create_systemd_service
    start_service
    print_instructions
}

# Run main function
main "$@"
