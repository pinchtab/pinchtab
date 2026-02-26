#!/usr/bin/env bash
set -euo pipefail

# Pinchtab Installer for macOS and Linux
# Usage: curl -fsSL https://pinchtab.com/install.sh | bash

BOLD='\033[1m'
ACCENT='\033[38;2;251;191;36m'      # yellow #fbbf24
INFO='\033[38;2;136;146;176m'       # muted #8892b0
SUCCESS='\033[38;2;0;229;204m'      # cyan #00e5cc
ERROR='\033[38;2;230;57;70m'        # red #e63946
MUTED='\033[38;2;90;100;128m'       # text-muted #5a6480
NC='\033[0m' # No Color

TAGLINE="12MB binary. Zero config. Accessibility-first browser control."

cleanup_tmpfiles() {
    local f
    for f in "${TMPFILES[@]:-}"; do
        rm -rf "$f" 2>/dev/null || true
    done
}
trap cleanup_tmpfiles EXIT

TMPFILES=()
LOGFILE=""

mktempfile() {
    local f
    f="$(mktemp)"
    TMPFILES+=("$f")
    echo "$f"
}

setup_logfile() {
    LOGFILE="$(mktempfile)"
}

ui_info() {
    local msg="$*"
    echo -e "${MUTED}·${NC} ${msg}"
}

ui_success() {
    local msg="$*"
    echo -e "${SUCCESS}✓${NC} ${msg}"
}

ui_error() {
    local msg="$*"
    echo -e "${ERROR}✗${NC} ${msg}"
}

ui_section() {
    local title="$1"
    echo ""
    echo -e "${ACCENT}${BOLD}${title}${NC}"
}

detect_os() {
    OS="unknown"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
    elif [[ "$OSTYPE" == "linux-gnu"* ]] || [[ -n "${WSL_DISTRO_NAME:-}" ]]; then
        OS="linux"
    fi

    if [[ "$OS" == "unknown" ]]; then
        ui_error "Unsupported operating system"
        echo "This installer supports macOS and Linux (including WSL)."
        exit 1
    fi

    ui_success "Detected: $OS"
}

print_banner() {
    echo -e "${ACCENT}${BOLD}"
    echo "  🦀 Pinchtab Installer"
    echo -e "${NC}${INFO}  ${TAGLINE}${NC}"
    echo ""
}

check_node() {
    if ! command -v node &> /dev/null; then
        ui_error "Node.js 18+ is required but not found"
        echo "Install from https://nodejs.org or via your package manager"
        exit 1
    fi

    local version
    version="$(node -v 2>/dev/null | cut -dv -f2 | cut -d. -f1)"
    if [[ -z "$version" || "$version" -lt 18 ]]; then
        ui_error "Node.js 18+ is required, but found $(node -v)"
        exit 1
    fi

    ui_success "Node.js $(node -v) found"
}

check_npm() {
    if ! command -v npm &> /dev/null; then
        ui_error "npm is required but not found"
        exit 1
    fi
    ui_success "npm $(npm -v) found"
}

install_pinchtab() {
    ui_section "Installing Pinchtab"
    
    if npm install -g pinchtab > "$LOGFILE" 2>&1; then
        ui_success "Pinchtab installed successfully"
        return 0
    else
        local exit_code=$?
        ui_error "npm install failed"
        echo ""
        
        # Check for permission error
        if grep -q "EACCES\|permission denied" "$LOGFILE"; then
            echo "This is a permission error. Try one of these:"
            echo ""
            echo "  Option 1: Use nvm (recommended)"
            echo "    curl https://github.com/nvm-sh/nvm/raw/master/install.sh | bash"
            echo "    nvm install node"
            echo "    npm install -g pinchtab"
            echo ""
            echo "  Option 2: Use user prefix (no sudo needed)"
            echo "    npm install -g --prefix \"\$HOME/.local\" pinchtab"
            echo "    export PATH=\"\$HOME/.local/bin:\$PATH\"  # Add to ~/.bashrc or ~/.zshrc"
            echo ""
            echo "  Option 3: Fix npm permissions globally"
            echo "    mkdir ~/.npm-global"
            echo "    npm config set prefix '~/.npm-global'"
            echo "    export PATH=~/.npm-global/bin:\$PATH  # Add to ~/.bashrc or ~/.zshrc"
            echo ""
        else
            echo "Install log:"
            cat "$LOGFILE"
        fi
        exit 1
    fi
}

verify_installation() {
    if ! command -v pinchtab &> /dev/null; then
        ui_error "Pinchtab binary not found in PATH after install"
        echo "Try: npm install -g pinchtab"
        exit 1
    fi

    local version
    version="$(pinchtab --version 2>/dev/null || echo 'unknown')"
    ui_success "Pinchtab ready: $version"
}

show_next_steps() {
    ui_section "Next steps"
    echo ""
    echo "  Start the server:"
    echo -e "    ${MUTED}pinchtab${NC}"
    echo ""
    echo "  In another terminal, test it:"
    echo -e "    ${MUTED}curl http://localhost:9867/health${NC}"
    echo ""
    echo "  Or navigate & snapshot:"
    echo -e "    ${MUTED}pinchtab nav https://httpbin.org/html${NC}"
    echo -e "    ${MUTED}pinchtab snap${NC}  (pipe to 'jq .count' if you have jq installed)"
    echo ""
    echo "  Documentation:"
    echo -e "    ${MUTED}https://pinchtab.com${NC}"
    echo ""
}

main() {
    print_banner
    setup_logfile
    
    detect_os
    check_node
    check_npm
    install_pinchtab
    verify_installation
    show_next_steps
    
    ui_section "Installation complete!"
    echo -e "Run ${ACCENT}${BOLD}pinchtab${NC} to start 🦀"
    echo ""
}

main "$@"
