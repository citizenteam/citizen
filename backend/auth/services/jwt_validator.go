package services

import (
	"backend/services"
	"os"
	"sync"
)

var (
	globalJWTValidator      *services.JWTValidator
	jwtValidatorInitialized bool
	jwtValidatorMutex       sync.Mutex
)

// GetJWTValidator returns the JWT validator instance (lazy init)
func GetJWTValidator() *services.JWTValidator {
	jwtValidatorMutex.Lock()
	defer jwtValidatorMutex.Unlock()

	if !jwtValidatorInitialized {
		jwksURL := os.Getenv("CITIZENAUTH_JWKS_URL")
		if jwksURL != "" {
			globalJWTValidator = services.NewJWTValidator(jwksURL)
		}
		jwtValidatorInitialized = true
	}

	return globalJWTValidator
}
