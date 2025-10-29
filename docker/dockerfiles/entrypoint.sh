#!/bin/sh
set -e

# Production SSH key setup
setup_ssh_keys() {
    if [ ! -d "/ssh_keys" ]; then
        echo "ERROR: /ssh_keys volume not mounted" >&2
        return 1
    fi

    echo "Setting up SSH keys for production..."

    # Create .ssh directory for appuser (check if not exists)
    if [ ! -d "/home/appuser/.ssh" ]; then
        mkdir -p /home/appuser/.ssh
        chmod 700 /home/appuser/.ssh
    fi

    # Copy SSH keys to appuser home (safer than mounting)
    if [ -f "/ssh_keys/id_rsa" ]; then
        cp /ssh_keys/id_rsa /home/appuser/.ssh/id_rsa
        chmod 600 /home/appuser/.ssh/id_rsa
        chown appuser:appgroup /home/appuser/.ssh/id_rsa 2>/dev/null || true
        echo "SSH private key configured"
    else
        echo "ERROR: SSH private key /ssh_keys/id_rsa is missing" >&2
        return 1
    fi

    if [ -f "/ssh_keys/id_rsa.pub" ]; then
        cp /ssh_keys/id_rsa.pub /home/appuser/.ssh/id_rsa.pub
        chmod 644 /home/appuser/.ssh/id_rsa.pub
        chown appuser:appgroup /home/appuser/.ssh/id_rsa.pub 2>/dev/null || true
        echo "SSH public key configured"
    else
        echo "ERROR: SSH public key /ssh_keys/id_rsa.pub is missing" >&2
        return 1
    fi

    # Set final ownership (run as root before switching to appuser)
    chown -R appuser:appgroup /home/appuser/.ssh 2>/dev/null || true
    echo "SSH keys setup completed"
}

# Run setup
setup_ssh_keys || exit 1

# Switch to appuser and run the application
exec su-exec appuser "$@"
