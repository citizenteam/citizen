package handlers

import (
	"html"
	"strings"
)

// HtmlEscapeForAttribute escapes a string for safe embedding in HTML attributes
// This prevents XSS attacks by escaping HTML entities and single quotes
func HtmlEscapeSingleQuotes(s string) string {
	// First escape standard HTML entities (<, >, &, ")
	escaped := html.EscapeString(s)
	// Then escape single quotes for attribute values using single quotes
	return strings.ReplaceAll(escaped, "'", "&#39;")
}

// Min returns the smaller of two integers (helper for Go versions < 1.21)
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
