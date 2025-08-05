package api

import (
	"context"
	"fmt"

	"backend/models"
)

// UserAPI provides user-related database operations

// CreateUser creates a new user
func (u *UserAPI) CreateUser(ctx context.Context, user *models.User) error {
	if err := ValidateArgs(user.Username, user.Password, user.Email); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		INSERT INTO users (username, password, email, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	now := GetCurrentTimestamp()
	err := QueryRow(ctx, query, user.Username, user.Password, user.Email, now, now).Scan(&user.ID)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by ID
func (u *UserAPI) GetUserByID(ctx context.Context, id int) (*models.User, error) {
	if err := ValidateArgs(id); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, username, password, email, github_id, github_username, 
		       github_access_token, github_connected, created_at, updated_at
		FROM users WHERE id = $1`

	user := &models.User{}
	err := QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Password, &user.Email,
		&user.GitHubID, &user.GitHubUsername, &user.GitHubAccessToken,
		&user.GitHubConnected, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByUsername retrieves a user by username
func (u *UserAPI) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	if err := ValidateArgs(username); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, username, password, email, github_id, github_username,
		       github_access_token, github_connected, created_at, updated_at
		FROM users WHERE username = $1`

	user := &models.User{}
	err := QueryRow(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.Password, &user.Email,
		&user.GitHubID, &user.GitHubUsername, &user.GitHubAccessToken,
		&user.GitHubConnected, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByGitHubID retrieves a user by GitHub ID
func (u *UserAPI) GetUserByGitHubID(ctx context.Context, githubID int) (*models.User, error) {
	if err := ValidateArgs(githubID); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, username, password, email, github_id, github_username,
		       github_access_token, github_connected, created_at, updated_at
		FROM users WHERE github_id = $1`

	user := &models.User{}
	err := QueryRow(ctx, query, githubID).Scan(
		&user.ID, &user.Username, &user.Password, &user.Email,
		&user.GitHubID, &user.GitHubUsername, &user.GitHubAccessToken,
		&user.GitHubConnected, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// UpdateUser updates an existing user
func (u *UserAPI) UpdateUser(ctx context.Context, user *models.User) error {
	if err := ValidateArgs(user.ID, user.Username, user.Email); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE users 
		SET username = $2, email = $3, github_id = $4, github_username = $5,
		    github_access_token = $6, github_connected = $7, updated_at = $8
		WHERE id = $1`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, user.ID, user.Username, user.Email,
		user.GitHubID, user.GitHubUsername, user.GitHubAccessToken,
		user.GitHubConnected, now)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// UpdateUserPassword updates a user's password
func (u *UserAPI) UpdateUserPassword(ctx context.Context, userID int, hashedPassword string) error {
	if err := ValidateArgs(userID, hashedPassword); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `UPDATE users SET password = $2, updated_at = $3 WHERE id = $1`
	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, userID, hashedPassword, now)
	if err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}

	return nil
}

// ConnectGitHub connects a user to GitHub
func (u *UserAPI) ConnectGitHub(ctx context.Context, userID int, githubID int, githubUsername, accessToken string) error {
	if err := ValidateArgs(userID, githubID, githubUsername, accessToken); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE users 
		SET github_id = $2, github_username = $3, github_access_token = $4, 
		    github_connected = true, updated_at = $5
		WHERE id = $1`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, userID, githubID, githubUsername, accessToken, now)
	if err != nil {
		return fmt.Errorf("failed to connect GitHub: %w", err)
	}

	return nil
}

// DisconnectGitHub disconnects a user from GitHub
func (u *UserAPI) DisconnectGitHub(ctx context.Context, userID int) error {
	if err := ValidateArgs(userID); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		UPDATE users 
		SET github_id = NULL, github_username = NULL, github_access_token = NULL,
		    github_connected = false, updated_at = $2
		WHERE id = $1`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, userID, now)
	if err != nil {
		return fmt.Errorf("failed to disconnect GitHub: %w", err)
	}

	return nil
}

// DeleteUser soft deletes a user
func (u *UserAPI) DeleteUser(ctx context.Context, userID int) error {
	if err := ValidateArgs(userID); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// In this case, we'll just set the user as inactive or remove sensitive data
	query := `
		UPDATE users 
		SET password = '', email = '', github_access_token = NULL, 
		    github_connected = false, updated_at = $2
		WHERE id = $1`

	now := GetCurrentTimestamp()
	_, err := Exec(ctx, query, userID, now)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// ListUsers retrieves all users (admin only)
func (u *UserAPI) ListUsers(ctx context.Context, limit, offset int) ([]models.User, error) {
	if err := ValidateArgs(limit, offset); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	query := `
		SELECT id, username, password, email, github_id, github_username,
		       github_access_token, github_connected, created_at, updated_at
		FROM users 
		ORDER BY created_at DESC 
		LIMIT $1 OFFSET $2`

	rows, err := Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		user := models.User{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.Password, &user.Email,
			&user.GitHubID, &user.GitHubUsername, &user.GitHubAccessToken,
			&user.GitHubConnected, &user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// UserExists checks if a user exists by username or email
func (u *UserAPI) UserExists(ctx context.Context, username, email string) (bool, error) {
	if err := ValidateArgs(username, email); err != nil {
		return false, fmt.Errorf("validation failed: %w", err)
	}

	query := `SELECT COUNT(*) FROM users WHERE username = $1 OR email = $2`
	var count int
	err := QueryRow(ctx, query, username, email).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return count > 0, nil
} 