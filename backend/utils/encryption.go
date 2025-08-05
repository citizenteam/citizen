package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
)

var (
	ErrMissingEncryptionKey = errors.New("ENCRYPTION_KEY environment variable is required for production")
	ErrInvalidEncryptionKey = errors.New("ENCRYPTION_KEY must be at least 16 characters long")
	encryptionKey []byte
)

// InitEncryption initializes and validates the encryption key at startup
func InitEncryption() error {
	keyStr := os.Getenv("ENCRYPTION_KEY")
	if keyStr == "" {
		return ErrMissingEncryptionKey
	}
	
	// Validate minimum key length
	if len(keyStr) < 16 {
		return ErrInvalidEncryptionKey
	}
	
	// Create a 32-byte key from the string
	hasher := sha256.New()
	hasher.Write([]byte(keyStr))
	encryptionKey = hasher.Sum(nil)
	
	log.Println("Encryption system initialized successfully")
	return nil
}

// getEncryptionKey returns the validated encryption key
func getEncryptionKey() ([]byte, error) {
	if encryptionKey == nil {
		return nil, ErrMissingEncryptionKey
	}
	return encryptionKey, nil
}

// EncryptString encrypts a string using AES-GCM
func EncryptString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	
	key, err := getEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("encryption key error: %w", err)
	}
	
	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	// Encrypt
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	
	// Encode to base64
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptString decrypts a string using AES-GCM
func DecryptString(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	
	key, err := getEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("encryption key error: %w", err)
	}
	
	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}
	
	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	
	// Create GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	
	// Check minimum length
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short: expected at least %d bytes, got %d", nonceSize, len(data))
	}
	
	// Extract nonce and ciphertext
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	
	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	
	return string(plaintext), nil
}

// EncryptedConfig represents encrypted configuration data
type EncryptedConfig struct {
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	RedirectURI   string
}

// EncryptConfig encrypts GitHub OAuth configuration
func EncryptConfig(config *EncryptedConfig) (map[string]string, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	
	encrypted := make(map[string]string)
	
	// Encrypt sensitive fields
	clientID, err := EncryptString(config.ClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt client ID: %w", err)
	}
	encrypted["client_id"] = clientID
	
	clientSecret, err := EncryptString(config.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt client secret: %w", err)
	}
	encrypted["client_secret"] = clientSecret
	
	webhookSecret, err := EncryptString(config.WebhookSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}
	encrypted["webhook_secret"] = webhookSecret
	
	// Redirect URI is not encrypted (not sensitive)
	encrypted["redirect_uri"] = config.RedirectURI
	
	return encrypted, nil
}

// DecryptConfig decrypts GitHub OAuth configuration
func DecryptConfig(encrypted map[string]string) (*EncryptedConfig, error) {
	if encrypted == nil {
		return nil, errors.New("encrypted config cannot be nil")
	}
	
	config := &EncryptedConfig{}
	
	// Decrypt sensitive fields
	clientID, err := DecryptString(encrypted["client_id"])
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client ID: %w", err)
	}
	config.ClientID = clientID
	
	clientSecret, err := DecryptString(encrypted["client_secret"])
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt client secret: %w", err)
	}
	config.ClientSecret = clientSecret
	
	webhookSecret, err := DecryptString(encrypted["webhook_secret"])
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt webhook secret: %w", err)
	}
	config.WebhookSecret = webhookSecret
	
	// Redirect URI is not encrypted
	config.RedirectURI = encrypted["redirect_uri"]
	
	return config, nil
}

// ValidateEncryptionSetup checks if encryption is properly configured
func ValidateEncryptionSetup() error {
	if encryptionKey == nil {
		return ErrMissingEncryptionKey
	}
	
	// Test encryption/decryption
	testData := "test-encryption-validation"
	encrypted, err := EncryptString(testData)
	if err != nil {
		return fmt.Errorf("encryption validation failed: %w", err)
	}
	
	decrypted, err := DecryptString(encrypted)
	if err != nil {
		return fmt.Errorf("decryption validation failed: %w", err)
	}
	
	if decrypted != testData {
		return errors.New("encryption/decryption validation failed: data mismatch")
	}
	
	return nil
} 