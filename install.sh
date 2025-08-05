#!/bin/bash

# ===================================================================
# CITIZEN - INSTALLATION SCRIPT
# ===================================================================
# This script clones the Citizen repository and runs the setup script.
# It can be executed via curl:
# curl -sSL https://raw.githubusercontent.com/citizenteam/citizen/main/install.sh | bash
# ===================================================================

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Configuration ---
REPO_URL="https://github.com/citizenteam/citizen.git"
CLONE_DIR="citizen"

# --- Colors for output ---
CYAN='\033[0;36m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# --- Helper functions ---
info() {
    echo -e "${CYAN}> $1${NC}"
}

success() {
    echo -e "${GREEN}✓ $1${NC}"
}

error() {
    echo -e "${RED}✗ $1${NC}" >&2
    exit 1
}

# --- Main Script ---

# 1. Check for Git
info "Checking for Git installation..."
if ! command -v git &> /dev/null; then
    error "Git is not installed. Please install Git and try again."
fi
success "Git is installed."

# 2. Clone or update the repository
if [ -d "$CLONE_DIR" ]; then
    info "Citizen directory already exists. Pulling the latest changes..."
    cd "$CLONE_DIR"
    git pull origin main || {
        info "Could not pull from 'origin main', trying default pull..."
        git pull
    }
    cd ..
else
    info "Cloning Citizen repository from GitHub..."
    git clone --depth 1 "$REPO_URL" "$CLONE_DIR"
fi
success "Repository is ready."

# 3. Change into the docker directory
info "Changing directory to citizen/docker..."
cd "$CLONE_DIR/docker"

# 4. Make the setup script executable
info "Making setup.sh executable..."
chmod +x setup.sh

# 5. Execute the setup script
info "Starting the Citizen setup script..."
echo "----------------------------------------------------"
./setup.sh < /dev/tty
echo "----------------------------------------------------"

success "Citizen setup process has been initiated."
info "Please follow the prompts from the setup script to complete the installation." 