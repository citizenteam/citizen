package handlers

import (
	"backend/services"
	"os"
	"sync"
)

var (
	globalJWTValidator     *services.JWTValidator
	jwtValidatorInitialized bool
	jwtValidatorMutex      sync.Mutex
)

// getJWTValidator returns the JWT validator instance (lazy init)
func getJWTValidator() *services.JWTValidator {
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

