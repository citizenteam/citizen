package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"backend/config"

	"golang.org/x/crypto/ssh"
)

var sshClient *ssh.Client

// testSSHConnection tests if the current SSH connection is working
func testSSHConnection() bool {
	if sshClient == nil {
		SSHDebugLog("testSSHConnection: sshClient is nil")
		return false
	}
	
	// Try to create a session to test the connection
	session, err := sshClient.NewSession()
	if err != nil {
		SSHDebugLog("testSSHConnection: NewSession failed: %v", err)
		return false
	}
	session.Close()
	SSHDebugLog("testSSHConnection: Session test successful")
	return true
}

// SSHConnect establishes SSH connection
func SSHConnect() error {
	SSHDebugLog("SSHConnect started...")
	
	// Test existing connection first
	if testSSHConnection() {
		SSHDebugLog("Current SSH connection is active, no need to reconnect")
		return nil
	}
	
	// Close broken connection if it exists
	if sshClient != nil {
		SSHDebugLog("Closing old SSH connection...")
		sshClient.Close()
		sshClient = nil
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	log.Printf("[SSH DEBUG] SSH Config loaded - Host: %s:%d, User: %s", cfg.SSHHost, cfg.SSHPort, cfg.SSHUser)

	// SSH connection configuration
	sshConfig := &ssh.ClientConfig{
		User: cfg.SSHUser,
		Auth: []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout: 10 * time.Second,
	}

	// Password authentication
	if cfg.SSHPassword != "" {
		log.Printf("[SSH DEBUG] SSH password found, adding password auth")
		sshConfig.Auth = append(sshConfig.Auth, ssh.Password(cfg.SSHPassword))
	} else {
		log.Printf("[SSH DEBUG] SSH password not found")
	}

	// SSH key authentication
	if cfg.SSHKeyPath != "" {
		log.Printf("[SSH DEBUG] SSH Key Path: %s", cfg.SSHKeyPath)
		keyPath := cfg.SSHKeyPath
		// Expand paths starting with ~
		if strings.HasPrefix(keyPath, "~") {
			home, err := os.UserHomeDir()
			if err == nil {
				keyPath = filepath.Join(home, keyPath[1:])
				log.Printf("[SSH DEBUG] ~ expanded: %s", keyPath)
			} else {
				log.Printf("[SSH DEBUG] ~ expansion error: %v", err)
			}
		}

		// Check SSH key file existence
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			log.Printf("[SSH DEBUG] SSH key file not found: %s", keyPath)
		} else {
			log.Printf("[SSH DEBUG] SSH key file found: %s", keyPath)
			
			key, err := ioutil.ReadFile(keyPath)
			if err != nil {
				log.Printf("[SSH DEBUG] SSH key read error: %v", err)
			} else {
				log.Printf("[SSH DEBUG] SSH key successfully read, %d bytes", len(key))
				
				signer, err := ssh.ParsePrivateKey(key)
				if err != nil {
					log.Printf("[SSH DEBUG] SSH key parse error: %v", err)
				} else {
					log.Printf("[SSH DEBUG] SSH key successfully parsed, adding public key auth")
					sshConfig.Auth = append(sshConfig.Auth, ssh.PublicKeys(signer))
				}
			}
		}
	} else {
		log.Printf("[SSH DEBUG] SSH key path not found")
	}

	log.Printf("[SSH DEBUG] Total %d auth methods found", len(sshConfig.Auth))
	for i, auth := range sshConfig.Auth {
		log.Printf("[SSH DEBUG] Auth method %d: %T", i+1, auth)
	}

	// Establish SSH connection with retry logic
	addr := fmt.Sprintf("%s:%d", cfg.SSHHost, cfg.SSHPort)
	log.Printf("[SSH DEBUG] Attempting SSH connection: %s", addr)
	
	// Retry connection up to 3 times with delay
	for i := 0; i < 3; i++ {
		log.Printf("[SSH DEBUG] SSH connection attempt %d/3...", i+1)
		sshClient, err = ssh.Dial("tcp", addr, sshConfig)
		if err == nil {
			log.Printf("[SSH DEBUG] SSH connection successful! (attempt %d)", i+1)
			break
		}
		log.Printf("[SSH DEBUG] SSH connection error (attempt %d): %v", i+1, err)
		if i < 2 { // Don't sleep on last attempt
			log.Printf("[SSH DEBUG] Waiting 2 seconds...")
			time.Sleep(2 * time.Second)
		}
	}
	
	if err != nil {
		log.Printf("[SSH DEBUG] SSH connection failed after 3 attempts!")
		return fmt.Errorf("SSH connection could not be established (after 3 attempts): %v", err)
	}

	log.Printf("[SSH DEBUG] SSH connection completely successful!")
	return nil
}

// SSHDisconnect closes the SSH connection
func SSHDisconnect() {
	if sshClient != nil {
		log.Printf("[SSH DEBUG] Closing SSH connection...")
		sshClient.Close()
		sshClient = nil
	}
}

// RunSSHCommand executes commands via SSH
func RunSSHCommand(command string) (string, error) {
	log.Printf("[SSH DEBUG] RunSSHCommand called: %s", command)
	
	// Check SSH connection and reconnect if necessary
	if err := SSHConnect(); err != nil {
		log.Printf("[SSH DEBUG] RunSSHCommand: SSH connection failed: %v", err)
		return "", err
	}

	// Open a new SSH session
	session, err := sshClient.NewSession()
	if err != nil {
		log.Printf("[SSH DEBUG] RunSSHCommand: First session opening error: %v", err)
		// Connection might be broken, try to reconnect
		SSHDisconnect()
		if err := SSHConnect(); err != nil {
			log.Printf("[SSH DEBUG] RunSSHCommand: Reconnection failed: %v", err)
			return "", fmt.Errorf("SSH reconnection failed: %v", err)
		}
		
		// Try creating session again
		session, err = sshClient.NewSession()
		if err != nil {
			log.Printf("[SSH DEBUG] RunSSHCommand: Second session opening error: %v", err)
			return "", fmt.Errorf("SSH session could not be opened: %v", err)
		}
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	log.Printf("[SSH DEBUG] Executing SSH command: %s", command)
	// Execute the command
	err = session.Run(command)
	if err != nil {
		errStr := stderr.String()
		log.Printf("[SSH DEBUG] SSH command error - stdout: %s, stderr: %s, err: %v", stdout.String(), errStr, err)
		if errStr != "" {
			return "", fmt.Errorf("%s: %v", errStr, err)
		}
		return "", err
	}

	result := stdout.String()
	log.Printf("[SSH DEBUG] SSH command successful - output: %s", result)
	return result, nil
} 
