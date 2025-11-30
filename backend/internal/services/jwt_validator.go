package services

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTValidator handles JWT validation with RS256 signature verification
type JWTValidator struct {
	jwksURL    string
	publicKey  *rsa.PublicKey
	keyMutex   sync.RWMutex
	lastUpdate time.Time
	httpClient *http.Client
}

// SSOClaims represents JWT claims from CitizenAuth
type SSOClaims struct {
	UserID         string  `json:"user_id"`
	Email          string  `json:"email"`
	Name           string  `json:"name"`
	OrganizationID *string `json:"organization_id"`
	Role           string  `json:"role"`
	IsSuperAdmin   bool    `json:"is_super_admin"`
	Fingerprint    string  `json:"fingerprint"`
	SessionID      string  `json:"session_id"`
	jwt.RegisteredClaims
}

// JWKS represents JSON Web Key Set structure
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kty string `json:"kty"` // Key Type (RSA)
	Use string `json:"use"` // Public Key Use (sig)
	Kid string `json:"kid"` // Key ID
	Alg string `json:"alg"` // Algorithm (RS256)
	N   string `json:"n"`   // Modulus
	E   string `json:"e"`   // Exponent
}

// NewJWTValidator creates a new JWT validator instance
func NewJWTValidator(jwksURL string) *JWTValidator {
	if jwksURL == "" {
		jwksURL = os.Getenv("CITIZENAUTH_JWKS_URL")
	}

	validator := &JWTValidator{
		jwksURL: jwksURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	// Initial public key fetch
	if err := validator.refreshPublicKey(); err != nil {
		log.Printf("⚠️  [JWT] Failed to fetch public key on init: %v", err)
		log.Println("⚠️  [JWT] Will retry on first validation request")
	}

	// Background refresh every 24 hours
	go validator.autoRefresh()

	return validator
}

// ValidateToken validates JWT token with RS256 signature verification
// This is a LOCAL operation - no network call to CitizenAuth
func (jv *JWTValidator) ValidateToken(tokenString string) (*SSOClaims, error) {
	// Get public key (from cache)
	jv.keyMutex.RLock()
	publicKey := jv.publicKey
	jv.keyMutex.RUnlock()

	if publicKey == nil {
		// Try to refresh if not available
		if err := jv.refreshPublicKey(); err != nil {
			return nil, fmt.Errorf("public key not available: %w", err)
		}
		jv.keyMutex.RLock()
		publicKey = jv.publicKey
		jv.keyMutex.RUnlock()
	}

	// Parse and validate JWT with RS256
	token, err := jwt.ParseWithClaims(tokenString, &SSOClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method is RS256
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token invalid")
	}

	claims, ok := token.Claims.(*SSOClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	// Additional expiry check
	if time.Now().After(claims.ExpiresAt.Time) {
		return nil, fmt.Errorf("token expired")
	}

	return claims, nil
}

// refreshPublicKey fetches public key from JWKS endpoint
func (jv *JWTValidator) refreshPublicKey() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", jv.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := jv.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("failed to decode JWKS: %w", err)
	}

	if len(jwks.Keys) == 0 {
		return fmt.Errorf("no keys in JWKS")
	}

	// Use first key (in production, match by kid)
	key := jwks.Keys[0]

	// Convert JWK to RSA public key
	publicKey, err := jv.jwkToRSAPublicKey(key)
	if err != nil {
		return fmt.Errorf("failed to convert JWK to RSA public key: %w", err)
	}

	// Update cache
	jv.keyMutex.Lock()
	jv.publicKey = publicKey
	jv.lastUpdate = time.Now()
	jv.keyMutex.Unlock()

	log.Printf("✅ [JWT] Public key refreshed from JWKS (kid: %s)", key.Kid)
	return nil
}

// jwkToRSAPublicKey converts JWK to RSA public key
func (jv *JWTValidator) jwkToRSAPublicKey(key JWK) (*rsa.PublicKey, error) {
	// Decode modulus (n)
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode exponent (e)
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert to big.Int
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	// Create RSA public key
	publicKey := &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}

	return publicKey, nil
}

// autoRefresh automatically refreshes public key every 24 hours
func (jv *JWTValidator) autoRefresh() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("🔄 [JWT] Auto-refreshing public key...")
		if err := jv.refreshPublicKey(); err != nil {
			log.Printf("❌ [JWT] Failed to auto-refresh public key: %v", err)
		}
	}
}

// GetLastUpdate returns when the public key was last updated
func (jv *JWTValidator) GetLastUpdate() time.Time {
	jv.keyMutex.RLock()
	defer jv.keyMutex.RUnlock()
	return jv.lastUpdate
}

