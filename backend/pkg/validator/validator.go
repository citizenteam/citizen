package validator

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	urlRegex   = regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	if email == "" {
		return &ValidationError{Field: "email", Message: "email is required"}
	}

	email = strings.TrimSpace(strings.ToLower(email))
	if !emailRegex.MatchString(email) {
		return &ValidationError{Field: "email", Message: "invalid email format"}
	}

	if len(email) > 254 {
		return &ValidationError{Field: "email", Message: "email is too long"}
	}

	return nil
}

// ValidateURL validates a URL
func ValidateURL(urlStr string) error {
	if urlStr == "" {
		return &ValidationError{Field: "url", Message: "url is required"}
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return &ValidationError{Field: "url", Message: "invalid URL format"}
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return &ValidationError{Field: "url", Message: "URL must use http or https scheme"}
	}

	if parsedURL.Host == "" {
		return &ValidationError{Field: "url", Message: "URL must have a host"}
	}

	return nil
}

// ValidateRequired validates that a field is not empty
func ValidateRequired(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{Field: field, Message: fmt.Sprintf("%s is required", field)}
	}
	return nil
}

// ValidateLength validates string length
func ValidateLength(field, value string, min, max int) error {
	length := len(strings.TrimSpace(value))
	if length < min {
		return &ValidationError{Field: field, Message: fmt.Sprintf("%s must be at least %d characters", field, min)}
	}
	if max > 0 && length > max {
		return &ValidationError{Field: field, Message: fmt.Sprintf("%s must be at most %d characters", field, max)}
	}
	return nil
}

// ValidateDomain validates a domain name
func ValidateDomain(domain string) error {
	if domain == "" {
		return &ValidationError{Field: "domain", Message: "domain is required"}
	}

	domain = strings.TrimSpace(strings.ToLower(domain))

	// Basic domain validation
	if len(domain) > 253 {
		return &ValidationError{Field: "domain", Message: "domain is too long"}
	}

	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return &ValidationError{Field: "domain", Message: "domain must have at least one dot"}
	}

	// Validate each part
	for _, part := range parts {
		if len(part) == 0 {
			return &ValidationError{Field: "domain", Message: "domain parts cannot be empty"}
		}
		if len(part) > 63 {
			return &ValidationError{Field: "domain", Message: "domain part is too long"}
		}
		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return &ValidationError{Field: "domain", Message: "domain part cannot start or end with hyphen"}
		}
	}

	return nil
}

// ValidateUsername validates a username
func ValidateUsername(username string) error {
	if username == "" {
		return &ValidationError{Field: "username", Message: "username is required"}
	}

	username = strings.TrimSpace(username)

	if len(username) < 3 {
		return &ValidationError{Field: "username", Message: "username must be at least 3 characters"}
	}

	if len(username) > 30 {
		return &ValidationError{Field: "username", Message: "username must be at most 30 characters"}
	}

	// Only alphanumeric, underscore, and hyphen
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !usernameRegex.MatchString(username) {
		return &ValidationError{Field: "username", Message: "username can only contain letters, numbers, underscore, and hyphen"}
	}

	return nil
}

// Sanitize sanitizes a string by trimming and removing dangerous characters
func Sanitize(input string) string {
	input = strings.TrimSpace(input)
	// Remove null bytes and control characters
	input = strings.ReplaceAll(input, "\x00", "")
	input = strings.ReplaceAll(input, "\r", "")
	return input
}

// SanitizeHTML removes HTML tags from a string
func SanitizeHTML(input string) string {
	// Simple HTML tag removal
	htmlTagRegex := regexp.MustCompile(`<[^>]*>`)
	return htmlTagRegex.ReplaceAllString(input, "")
}
