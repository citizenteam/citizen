package database

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RunMigrations runs all pending migrations
func RunMigrations() error {
	// Create migrations directory if it doesn't exist
	migrationsDir := "migrations"
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		err := os.MkdirAll(migrationsDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create migrations directory: %w", err)
		}
	}

	// Get all migration files
	files, err := ioutil.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Filter and sort .sql files
	var migrationFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sql") {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Create schema_migrations table if it doesn't exist
	err = createSchemaMigrationsTable()
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	// Run each migration
	for _, filename := range migrationFiles {
		version := strings.TrimSuffix(filename, ".sql")
		
		// Check if migration already applied
		applied, err := isMigrationApplied(version)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", version, err)
		}
		
		if applied {
			log.Printf("[MIGRATION] ✅ Migration %s already applied, skipping", version)
			continue
		}

		// Read and execute migration file
		log.Printf("[MIGRATION] 🚀 Running migration %s", version)
		err = executeMigration(filepath.Join(migrationsDir, filename), version)
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", version, err)
		}
		
		log.Printf("[MIGRATION] ✅ Migration %s completed successfully", version)
	}

	log.Printf("[MIGRATION] 🎉 All migrations completed successfully")
	return nil
}

// createSchemaMigrationsTable creates the schema_migrations table if it doesn't exist
func createSchemaMigrationsTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := DB.Exec(context.Background(), query)
	return err
}

// isMigrationApplied checks if a migration has already been applied
func isMigrationApplied(version string) (bool, error) {
	var count int
	err := DB.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM schema_migrations WHERE version = $1",
		version,
	).Scan(&count)
	
	if err != nil {
		return false, err
	}
	
	return count > 0, nil
}

// executeMigration executes a migration file
func executeMigration(filePath, version string) error {
	// Read migration file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	// Execute migration SQL
	_, err = DB.Exec(context.Background(), string(content))
	if err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration as applied
	_, err = DB.Exec(context.Background(),
		"INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING",
		version,
	)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return nil
}

// GetMigrationStatus returns the status of all migrations
func GetMigrationStatus() ([]MigrationStatus, error) {
	// Get all migration files
	migrationsDir := "migrations"
	files, err := ioutil.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var migrationFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sql") {
			migrationFiles = append(migrationFiles, strings.TrimSuffix(file.Name(), ".sql"))
		}
	}
	sort.Strings(migrationFiles)

	// Get applied migrations
	rows, err := DB.Query(context.Background(),
		"SELECT version, applied_at FROM schema_migrations ORDER BY version",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	appliedMigrations := make(map[string]string)
	for rows.Next() {
		var version, appliedAt string
		if err := rows.Scan(&version, &appliedAt); err != nil {
			continue
		}
		appliedMigrations[version] = appliedAt
	}

	// Build status list
	var status []MigrationStatus
	for _, migration := range migrationFiles {
		appliedAt, applied := appliedMigrations[migration]
		status = append(status, MigrationStatus{
			Version:   migration,
			Applied:   applied,
			AppliedAt: appliedAt,
		})
	}

	return status, nil
}

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	Version   string `json:"version"`
	Applied   bool   `json:"applied"`
	AppliedAt string `json:"applied_at,omitempty"`
}

// ForceMigration forces a migration to be marked as applied (dangerous!)
func ForceMigration(version string) error {
	_, err := DB.Exec(context.Background(),
		"INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING",
		version,
	)
	return err
}

// RollbackMigration removes a migration from the applied list (doesn't undo changes!)
func RollbackMigration(version string) error {
	_, err := DB.Exec(context.Background(),
		"DELETE FROM schema_migrations WHERE version = $1",
		version,
	)
	return err
} 