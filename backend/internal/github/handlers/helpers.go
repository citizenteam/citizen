package handlers

import "strings"

// htmlEscapeSingleQuotes escapes single quotes for embedding JSON in HTML attribute
func HtmlEscapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "&#39;")
}

// Min returns the smaller of two integers (helper for Go versions < 1.21)
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
