package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB holds the database connection pool
var DB *pgxpool.Pool

// InitDB initializes the database connection for the API package
func InitDB(db *pgxpool.Pool) {
	DB = db
}

// errorRow implements pgx.Row interface to return errors when DB is not available
type errorRow struct {
	err error
}

func (r *errorRow) Scan(dest ...interface{}) error {
	return r.err
}

// safeRecover handles panic recovery and returns appropriate error
func safeRecover(operation string) error {
	if r := recover(); r != nil {
		stack := debug.Stack()
		log.Printf("PANIC RECOVERED in %s: %v\nStack trace:\n%s", operation, r, stack)
		
		// Convert panic to error
		switch v := r.(type) {
		case error:
			return fmt.Errorf("panic in %s: %w", operation, v)
		case string:
			return fmt.Errorf("panic in %s: %s", operation, v)
		default:
			return fmt.Errorf("panic in %s: %v", operation, r)
		}
	}
	return nil
}

// QueryRow executes a query that returns a single row with panic recovery
func QueryRow(ctx context.Context, query string, args ...interface{}) pgx.Row {
	defer func() {
		if panicErr := safeRecover("QueryRow"); panicErr != nil {
			log.Printf("QueryRow failed: %v", panicErr)
			// Return a row that will return the error when scanned
		}
	}()
	
	if DB == nil {
		log.Printf("QueryRow: database connection not initialized")
		// Return a mock row that will return error when scanned
		// This is a workaround since we can't return nil from this function signature
		return &errorRow{err: errors.New("database connection not initialized")}
	}
	
	// Validate arguments (log warning but don't fail)
	if err := ValidateArgs(args...); err != nil {
		log.Printf("QueryRow argument validation warning: %v", err)
	}
	
	return DB.QueryRow(ctx, query, args...)
}

// QueryRowSafe executes a query that returns a single row with full error handling
func QueryRowSafe(ctx context.Context, query string, args ...interface{}) (row pgx.Row, err error) {
	defer func() {
		if panicErr := safeRecover("QueryRowSafe"); panicErr != nil {
			err = panicErr
			row = nil
		}
	}()
	
	if DB == nil {
		return nil, errors.New("database connection not initialized")
	}
	
	// Validate arguments
	if err := ValidateArgs(args...); err != nil {
		return nil, fmt.Errorf("argument validation failed: %w", err)
	}
	
	row = DB.QueryRow(ctx, query, args...)
	return row, nil
}

// Query executes a query that returns multiple rows with panic recovery
func Query(ctx context.Context, query string, args ...interface{}) (rows pgx.Rows, err error) {
	defer func() {
		if panicErr := safeRecover("Query"); panicErr != nil {
			err = panicErr
			if rows != nil {
				rows.Close()
			}
			rows = nil
		}
	}()
	
	if DB == nil {
		return nil, errors.New("database connection not initialized")
	}
	
	// Validate arguments
	if err := ValidateArgs(args...); err != nil {
		return nil, fmt.Errorf("argument validation failed: %w", err)
	}
	
	rows, err = DB.Query(ctx, query, args...)
	return rows, err
}

// Exec executes a query that doesn't return rows with panic recovery
func Exec(ctx context.Context, query string, args ...interface{}) (result pgconn.CommandTag, err error) {
	defer func() {
		if panicErr := safeRecover("Exec"); panicErr != nil {
			err = panicErr
			result = pgconn.CommandTag{}
		}
	}()
	
	if DB == nil {
		return pgconn.CommandTag{}, errors.New("database connection not initialized")
	}
	
	// Validate arguments
	if err := ValidateArgs(args...); err != nil {
		return pgconn.CommandTag{}, fmt.Errorf("argument validation failed: %w", err)
	}
	
	result, err = DB.Exec(ctx, query, args...)
	return result, err
}

// Transaction executes a function within a database transaction with enhanced panic recovery
func Transaction(ctx context.Context, fn func(pgx.Tx) error) (err error) {
	defer func() {
		if panicErr := safeRecover("Transaction"); panicErr != nil {
			err = panicErr
		}
	}()
	
	if DB == nil {
		return errors.New("database connection not initialized")
	}
	
	if fn == nil {
		return errors.New("transaction function cannot be nil")
	}
	
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	defer func() {
		if p := recover(); p != nil {
			// Rollback on panic
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				log.Printf("Failed to rollback transaction after panic: %v", rollbackErr)
			}
			
			// Re-handle the panic through our recovery mechanism
			panic(p)
		} else if err != nil {
			// Rollback on error
			if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
				log.Printf("Failed to rollback transaction after error: %v", rollbackErr)
			}
		} else {
			// Commit if no error
			if commitErr := tx.Commit(ctx); commitErr != nil {
				err = fmt.Errorf("failed to commit transaction: %w", commitErr)
			}
		}
	}()
	
	err = fn(tx)
	return err
}

// SafeOperation wraps any database operation with panic recovery
func SafeOperation(operation string, fn func() error) error {
	defer func() {
		if panicErr := safeRecover(operation); panicErr != nil {
			log.Printf("Database operation '%s' failed with panic: %v", operation, panicErr)
		}
	}()
	
	if fn == nil {
		return fmt.Errorf("operation function cannot be nil for: %s", operation)
	}
	
	return fn()
}

// Common helper functions

// GetCurrentTimestamp returns current timestamp for database operations
func GetCurrentTimestamp() time.Time {
	return time.Now()
}

// ValidateArgs validates arguments to prevent SQL injection with enhanced security
func ValidateArgs(args ...interface{}) error {
	for i, arg := range args {
		if arg == nil {
			continue
		}
		
		// Check for potentially dangerous strings
		if str, ok := arg.(string); ok {
			if containsDangerousSQL(str) {
				return fmt.Errorf("argument %d contains potentially dangerous SQL pattern: %s", i, str)
			}
			
			// Check for excessively long strings that might cause issues
			if len(str) > 10000 {
				return fmt.Errorf("argument %d is too long (%d characters), maximum allowed: 10000", i, len(str))
			}
		}
	}
	return nil
}

// containsDangerousSQL checks for dangerous SQL patterns with enhanced detection
func containsDangerousSQL(s string) bool {
	dangerousPatterns := []string{
		"DROP TABLE", "DELETE FROM", "TRUNCATE", "ALTER TABLE",
		"CREATE TABLE", "INSERT INTO", "UPDATE SET", "GRANT",
		"REVOKE", "EXEC", "EXECUTE", "UNION SELECT",
		"' OR '1'='1", "' OR 1=1", "'; DROP", "'; DELETE",
		"'; UPDATE", "'; INSERT", "'; ALTER", "'; CREATE",
		"SCRIPT", "JAVASCRIPT", "VBSCRIPT", "ONLOAD", "ONERROR",
		"EVAL(", "EXPRESSION(", "URL(", "IMPORT",
	}
	
	upperS := strings.ToUpper(strings.TrimSpace(s))
	for _, pattern := range dangerousPatterns {
		if strings.Contains(upperS, pattern) {
			return true
		}
	}
	
	// Check for multiple consecutive special characters that might indicate injection
	specialChars := []string{"''", "\"\"", ";;", "--", "/*", "*/", "@@"}
	for _, chars := range specialChars {
		if strings.Contains(s, chars) {
			return true
		}
	}
	
	return false
}

// HealthCheck performs a simple database health check with panic recovery
func HealthCheck(ctx context.Context) error {
	return SafeOperation("HealthCheck", func() error {
		if DB == nil {
			return errors.New("database connection not initialized")
		}
		
		// Simple ping to check database connectivity
		err := DB.Ping(ctx)
		if err != nil {
			return fmt.Errorf("database ping failed: %w", err)
		}
		
		return nil
	})
}

// API struct definitions
type UserAPI struct{}
type AppAPI struct{}
type DeploymentAPI struct{}
type GitHubAPI struct{}
type ActivityAPI struct{}
type SettingsAPI struct{}

// Main API struct that implements all operations
type API struct{}

// Database API Collections - All operations are exposed through these interfaces

// Users provides user-related database operations
var Users = &UserAPI{}

// Apps provides app-related database operations  
var Apps = &AppAPI{}

// Deployments provides deployment-related database operations
var Deployments = &DeploymentAPI{}

// GitHub provides GitHub-related database operations
var GitHub = &GitHubAPI{}

// Activities provides activity-related database operations
var Activities = &API{}

// Settings provides settings-related database operations
var Settings = &SettingsAPI{} 