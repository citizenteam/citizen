package database

import (
	"backend/utils"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type CitizenauthInstanceConfig struct {
	InstanceUUID   uuid.UUID
	OrganizationID uuid.UUID
	Domain         string
	CitizenauthURL string
	APIKey         string
	APIKeyPrefix   string
	WebhookSecret  string
}

type CitizenauthInstance struct {
	InstanceUUID           uuid.UUID
	OrganizationID         uuid.UUID
	Domain                 *string
	CitizenauthURL         *string
	APIKeyHash             *string
	APIKeyPrefix           *string
	APIKeyEncrypted        *string
	WebhookSecretEncrypted *string
	Status                 string
}

var (
	ErrCitizenauthInstanceNotFound = errors.New("citizenauth instance not found")
)

// UpsertCitizenauthInstance stores the CitizenAuth instance configuration and secrets.
func UpsertCitizenauthInstance(ctx context.Context, cfg *CitizenauthInstanceConfig) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	if cfg.InstanceUUID == uuid.Nil {
		return errors.New("instance uuid is required")
	}
	if cfg.OrganizationID == uuid.Nil {
		return errors.New("organization uuid is required")
	}
	if cfg.APIKey == "" {
		return errors.New("api key is required")
	}
	if cfg.WebhookSecret == "" {
		return errors.New("webhook secret is required")
	}

	apiKeyHash, err := utils.HashPassword(cfg.APIKey)
	if err != nil {
		return fmt.Errorf("failed to hash api key: %w", err)
	}

	apiKeyEncrypted, err := utils.EncryptString(cfg.APIKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt api key: %w", err)
	}

	webhookSecretEncrypted, err := utils.EncryptString(cfg.WebhookSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}

	apiKeyPrefix := cfg.APIKeyPrefix
	if apiKeyPrefix == "" {
		apiKeyPrefix = cfg.APIKey
		if len(apiKeyPrefix) > 8 {
			apiKeyPrefix = apiKeyPrefix[:8]
		}
	}

	query := `
        INSERT INTO citizenauth_instances (
            instance_uuid,
            organization_id,
            domain,
            citizenauth_url,
            api_key_hash,
            api_key_prefix,
            api_key_encrypted,
            webhook_secret_encrypted,
            status,
            registered_at,
            updated_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',NOW(),NOW())
        ON CONFLICT (instance_uuid)
        DO UPDATE SET
            organization_id = EXCLUDED.organization_id,
            domain = EXCLUDED.domain,
            citizenauth_url = EXCLUDED.citizenauth_url,
            api_key_hash = EXCLUDED.api_key_hash,
            api_key_prefix = EXCLUDED.api_key_prefix,
            api_key_encrypted = EXCLUDED.api_key_encrypted,
            webhook_secret_encrypted = EXCLUDED.webhook_secret_encrypted,
            status = 'active',
            updated_at = NOW();
    `

	_, err = DB.Exec(ctx, query,
		cfg.InstanceUUID,
		cfg.OrganizationID,
		nullString(cfg.Domain),
		nullString(cfg.CitizenauthURL),
		apiKeyHash,
		apiKeyPrefix,
		apiKeyEncrypted,
		webhookSecretEncrypted,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert citizenauth instance: %w", err)
	}

	return nil
}

func nullString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

// GetCitizenauthInstanceByPrefix returns instance configuration matched by API key prefix.
func GetCitizenauthInstanceByPrefix(ctx context.Context, prefix string) (*CitizenauthInstance, error) {
	query := `
        SELECT instance_uuid, organization_id, domain, citizenauth_url,
               api_key_hash, api_key_prefix, api_key_encrypted, webhook_secret_encrypted, status
        FROM citizenauth_instances
        WHERE api_key_prefix = $1 AND status = 'active'
    `

	row := DB.QueryRow(ctx, query, prefix)
	instance := &CitizenauthInstance{}
	var domain, citizenauthURL, apiKeyHash, apiKeyPrefix, apiKeyEncrypted, webhookEncrypted *string
	if err := row.Scan(
		&instance.InstanceUUID,
		&instance.OrganizationID,
		&domain,
		&citizenauthURL,
		&apiKeyHash,
		&apiKeyPrefix,
		&apiKeyEncrypted,
		&webhookEncrypted,
		&instance.Status,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCitizenauthInstanceNotFound
		}
		return nil, fmt.Errorf("failed to load citizenauth instance: %w", err)
	}

	instance.Domain = domain
	instance.CitizenauthURL = citizenauthURL
	instance.APIKeyHash = apiKeyHash
	instance.APIKeyPrefix = apiKeyPrefix
	instance.APIKeyEncrypted = apiKeyEncrypted
	instance.WebhookSecretEncrypted = webhookEncrypted

	return instance, nil
}

// GetCitizenauthInstanceByUUID returns the instance configuration for a specific instance UUID.
func GetCitizenauthInstanceByUUID(ctx context.Context, instanceUUID uuid.UUID) (*CitizenauthInstance, error) {
	query := `
        SELECT instance_uuid, organization_id, domain, citizenauth_url,
               api_key_hash, api_key_prefix, api_key_encrypted, webhook_secret_encrypted, status
        FROM citizenauth_instances
        WHERE instance_uuid = $1
    `

	row := DB.QueryRow(ctx, query, instanceUUID)
	instance := &CitizenauthInstance{}
	var domain, citizenauthURL, apiKeyHash, apiKeyPrefix, apiKeyEncrypted, webhookEncrypted *string
	if err := row.Scan(
		&instance.InstanceUUID,
		&instance.OrganizationID,
		&domain,
		&citizenauthURL,
		&apiKeyHash,
		&apiKeyPrefix,
		&apiKeyEncrypted,
		&webhookEncrypted,
		&instance.Status,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCitizenauthInstanceNotFound
		}
		return nil, fmt.Errorf("failed to load citizenauth instance: %w", err)
	}

	instance.Domain = domain
	instance.CitizenauthURL = citizenauthURL
	instance.APIKeyHash = apiKeyHash
	instance.APIKeyPrefix = apiKeyPrefix
	instance.APIKeyEncrypted = apiKeyEncrypted
	instance.WebhookSecretEncrypted = webhookEncrypted

	return instance, nil
}

// GetCitizenauthWebhookSecret returns decrypted webhook secret and organization ID.
func GetCitizenauthWebhookSecret(ctx context.Context, instanceUUID uuid.UUID) (string, uuid.UUID, error) {
	instance, err := GetCitizenauthInstanceByUUID(ctx, instanceUUID)
	if err != nil {
		return "", uuid.Nil, err
	}
	if instance.WebhookSecretEncrypted == nil || *instance.WebhookSecretEncrypted == "" {
		return "", uuid.Nil, fmt.Errorf("webhook secret not configured")
	}
	secret, err := utils.DecryptString(*instance.WebhookSecretEncrypted)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("failed to decrypt webhook secret: %w", err)
	}
	return secret, instance.OrganizationID, nil
}

// DeleteCitizenauthInstance removes the instance config and cleans related mappings.
func DeleteCitizenauthInstance(ctx context.Context, instanceUUID uuid.UUID) (uuid.UUID, error) {
	tx, err := DB.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var organizationID uuid.UUID
	queryOrg := `SELECT organization_id FROM citizenauth_instances WHERE instance_uuid = $1`
	if err = tx.QueryRow(ctx, queryOrg, instanceUUID).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrCitizenauthInstanceNotFound
			return uuid.Nil, err
		}
		return uuid.Nil, fmt.Errorf("failed to lookup instance: %w", err)
	}

	if _, err = tx.Exec(ctx, `DELETE FROM app_permissions WHERE organization_id = $1`, organizationID); err != nil {
		return uuid.Nil, fmt.Errorf("failed to delete app permissions: %w", err)
	}

	if _, err = tx.Exec(ctx, `DELETE FROM citizenauth_user_mapping WHERE organization_id = $1`, organizationID); err != nil {
		return uuid.Nil, fmt.Errorf("failed to delete user mappings: %w", err)
	}

	if _, err = tx.Exec(ctx, `DELETE FROM citizenauth_instances WHERE instance_uuid = $1`, instanceUUID); err != nil {
		return uuid.Nil, fmt.Errorf("failed to delete instance record: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("failed to commit instance deletion: %w", err)
	}

	return organizationID, nil
}

// Utility to update last sync timestamp after successful operations.
func UpdateCitizenauthInstanceSync(ctx context.Context, instanceUUID uuid.UUID, t time.Time) error {
	query := `UPDATE citizenauth_instances SET last_sync_at = $2, updated_at = NOW() WHERE instance_uuid = $1`
	if _, err := DB.Exec(ctx, query, instanceUUID, t); err != nil {
		return fmt.Errorf("failed to update instance sync: %w", err)
	}
	return nil
}
