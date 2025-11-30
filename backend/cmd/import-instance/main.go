package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"backend/internal/database"
	"backend/internal/utils"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

const globalAppID = "__all__"

var errInstanceAlreadyRegistered = errors.New("citizenauth instance already registered")

// BootstrapBundle models the data returned by CitizenAuth during server registration.
type BootstrapBundle struct {
	InstanceID                  string `json:"instance_id"`
	OrganizationID              string `json:"organization_id"`
	Name                        string `json:"name"`
	Domain                      string `json:"domain"`
	APIKey                      string `json:"api_key"`
	APIKeyPrefix                string `json:"api_key_prefix"`
	WebhookSecret               string `json:"webhook_secret"`
	RegistrationToken           string `json:"registration_token"`
	RegistrationTokenExpiresAt  string `json:"registration_token_expires_at"`
	HandshakeURL                string `json:"handshake_url"`
	HandshakeExampleDescription string `json:"handshake_example_challenge_note"`
	RequestedBy                 string `json:"requested_by"`
	RequestedByEmail            string `json:"requested_by_email"`
	RequestedByName             string `json:"requested_by_name"`
}

func main() {
	var (
		bundlePath  string
		envFile     string
		composeFile string
		composeSvc  string
		skipRestart bool
	)

	flag.StringVar(&bundlePath, "bundle", "", "Path to CitizenAuth bootstrap bundle JSON (required)")
	flag.StringVar(&envFile, "env", "../docker/.env", "Path to Citizen Docker .env file")
	flag.StringVar(&composeFile, "compose", "./docker/docker-compose.prod.yml", "Path to docker-compose file")
	flag.StringVar(&composeSvc, "service", "api", "Docker compose service name to restart")
	flag.BoolVar(&skipRestart, "skip-restart", false, "Skip restarting the docker service after import")
	flag.Parse()

	if bundlePath == "" {
		exitWithError(errors.New("bundle path is required"))
	}

	bundle, err := loadBundle(bundlePath)
	if err != nil {
		exitWithError(fmt.Errorf("load bundle: %w", err))
	}

	if err := performCitizenauthHandshake(bundle); err != nil {
		if errors.Is(err, errInstanceAlreadyRegistered) {
			fmt.Println("ℹ️  CitizenAuth already has this instance registered; skipping handshake.")
		} else {
			exitWithError(fmt.Errorf("citizenauth handshake: %w", err))
		}
	}

	if err := storeSecretsLocally(bundle, envFile); err != nil {
		exitWithError(fmt.Errorf("store secrets in citizen: %w", err))
	}

	if err := syncInstanceDomains(bundle); err != nil {
		fmt.Printf("⚠️  Failed to sync instance domains: %v\n", err)
	}

	// Sync permission to Citizen via webhook (self-call)
	if bundle.RequestedBy != "" {
		if err := sendPermissionGrantedWebhook(bundle, globalAppID, "admin"); err != nil {
			fmt.Printf("⚠️  Failed to sync permission webhook: %v\n", err)
			// Don't fail - permission is already saved locally in DB
		} else {
			fmt.Printf("✅ Permission synced: user=%s, app=%s, role=admin\n", bundle.RequestedBy, globalAppID)
		}
	}

	if err := updateEnvFile(envFile, bundle); err != nil {
		exitWithError(fmt.Errorf("update env file: %w", err))
	}

	if !skipRestart {
		if err := restartService(composeFile, envFile, composeSvc); err != nil {
			exitWithError(fmt.Errorf("restart service: %w", err))
		}
	}

	fmt.Println("✅ Citizen instance import completed successfully")
}

func loadBundle(path string) (*BootstrapBundle, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	try := func(data []byte, out interface{}) bool {
		if err := json.Unmarshal(data, out); err != nil {
			return false
		}
		switch b := out.(type) {
		case *BootstrapBundle:
			return b.InstanceID != "" && b.APIKey != ""
		case *struct {
			BootstrapBundle BootstrapBundle `json:"bootstrap_bundle"`
		}:
			return b.BootstrapBundle.InstanceID != ""
		case *struct {
			Data struct {
				BootstrapBundle BootstrapBundle `json:"bootstrap_bundle"`
			} `json:"data"`
		}:
			return b.Data.BootstrapBundle.InstanceID != ""
		}
		return false
	}

	var bundle BootstrapBundle
	if try(file, &bundle) {
		return &bundle, nil
	}

	var wrap struct {
		BootstrapBundle BootstrapBundle `json:"bootstrap_bundle"`
	}
	if try(file, &wrap) {
		return &wrap.BootstrapBundle, nil
	}

	var outer struct {
		Data struct {
			BootstrapBundle BootstrapBundle `json:"bootstrap_bundle"`
		} `json:"data"`
	}
	if try(file, &outer) {
		return &outer.Data.BootstrapBundle, nil
	}

	return nil, errors.New("file does not contain a bootstrap bundle")
}

func sendPermissionGrantedWebhook(bundle *BootstrapBundle, appID, role string) error {
	// Build Citizen webhook URL (local call)
	citizenDomain := bundle.Domain
	if citizenDomain == "" {
		return fmt.Errorf("citizen domain not available in bundle")
	}

	webhookURL := fmt.Sprintf("https://%s/api/v1/service/webhooks/permission-update", citizenDomain)

	timestamp := time.Now().Unix()
	payload := map[string]interface{}{
		"event":           "permission.granted",
		"user_id":         bundle.RequestedBy,
		"organization_id": bundle.OrganizationID,
		"app_id":          appID,
		"role":            role,
		"granted_by":      bundle.RequestedBy,
		"timestamp":       timestamp,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Compute HMAC signature
	message := fmt.Sprintf("%d.%s", timestamp, string(body))
	h := hmac.New(sha256.New, []byte(bundle.WebhookSecret))
	h.Write([]byte(message))
	signature := "sha256=" + hex.EncodeToString(h.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", timestamp))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	return nil
}

func performCitizenauthHandshake(bundle *BootstrapBundle) error {
	nonce := randomHex(32)
	challenge := computeChallenge(bundle.APIKey, nonce)

	payload := map[string]string{
		"registration_token": bundle.RegistrationToken,
		"nonce":              nonce,
		"challenge":          challenge,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, bundle.HandshakeURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusConflict {
			return errInstanceAlreadyRegistered
		}
		return fmt.Errorf("citizenauth handshake failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	return nil
}

func storeSecretsLocally(bundle *BootstrapBundle, envFile string) error {
	// Attempt to load ENV from the provided path first, then fall back to common defaults.
	// Paths optimized for running from Citizen root
	if envFile != "" {
		_ = godotenv.Overload(envFile)
	}
	_ = godotenv.Overload("./docker/.env")
	_ = godotenv.Overload("../docker/.env")
	_ = godotenv.Overload("../../docker/.env")

	ensureEncryptionKey := func(path string) {
		if os.Getenv("ENCRYPTION_KEY") != "" {
			return
		}
		if val, err := readEnvValue(path, "ENCRYPTION_KEY"); err == nil && val != "" {
			os.Setenv("ENCRYPTION_KEY", val)
		}
	}

	if envFile != "" {
		ensureEncryptionKey(envFile)
	}
	ensureEncryptionKey("./docker/.env")
	ensureEncryptionKey("../docker/.env")
	ensureEncryptionKey("../../docker/.env")

	if os.Getenv("ENCRYPTION_KEY") == "" {
		return fmt.Errorf("encryption key missing: define ENCRYPTION_KEY in docker/.env or export it before running the import script")
	}

	database.ConnectDB()
	defer database.CloseDB()

	if err := utils.InitEncryption(); err != nil {
		return fmt.Errorf("initialize encryption: %w", err)
	}

	instanceID, err := uuid.Parse(bundle.InstanceID)
	if err != nil {
		return fmt.Errorf("invalid instance_id: %w", err)
	}

	organizationID, err := uuid.Parse(bundle.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization_id: %w", err)
	}

	cfg := &database.CitizenauthInstanceConfig{
		InstanceUUID:   instanceID,
		OrganizationID: organizationID,
		Domain:         bundle.Domain,
		CitizenauthURL: baseURL(bundle.HandshakeURL),
		APIKey:         bundle.APIKey,
		APIKeyPrefix:   bundle.APIKeyPrefix,
		WebhookSecret:  bundle.WebhookSecret,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ensureCitizenauthSchema(ctx); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}

	if err := database.UpsertCitizenauthInstance(ctx, cfg); err != nil {
		return fmt.Errorf("upsert citizen auth instance: %w", err)
	}

	if bundle.RequestedBy != "" {
		if bundle.RequestedByEmail != "" {
			var localUserID int
			if err := database.DB.QueryRow(ctx,
				`SELECT get_or_create_local_user($1, $2, $3, $4)`,
				bundle.RequestedBy,
				bundle.RequestedByEmail,
				bundle.RequestedByName,
				organizationID,
			).Scan(&localUserID); err != nil {
				return fmt.Errorf("ensure admin user mapping: %w", err)
			}
		}

		if _, err := database.DB.Exec(ctx,
			`SELECT grant_app_permission($1, $2, $3, $4, $5)`,
			bundle.RequestedBy,
			organizationID,
			globalAppID,
			"admin",
			bundle.RequestedBy,
		); err != nil {
			return fmt.Errorf("grant admin permission: %w", err)
		}
	}

	return nil
}

func ensureCitizenauthSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS citizenauth_instances (
			id SERIAL PRIMARY KEY,
			instance_uuid UUID NOT NULL UNIQUE,
			organization_id UUID NOT NULL,
			domain VARCHAR(500),
			citizenauth_url VARCHAR(500),
			api_key_hash TEXT,
			api_key_prefix VARCHAR(20),
			api_key_encrypted TEXT,
			webhook_secret_encrypted TEXT,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			registered_at TIMESTAMPTZ,
			last_sync_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS api_key_hash TEXT`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS api_key_prefix VARCHAR(20)`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS api_key_encrypted TEXT`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS webhook_secret_encrypted TEXT`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS status VARCHAR(20)`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS registered_at TIMESTAMPTZ`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ DEFAULT NOW()`,
		`ALTER TABLE citizenauth_instances ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW()`,
		`ALTER TABLE app_permissions ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ DEFAULT NOW()`,
		`ALTER TABLE app_permissions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW()`,
		`CREATE OR REPLACE FUNCTION check_app_permission(
			p_user_id VARCHAR,
			p_organization_id UUID,
			p_app_id VARCHAR,
			p_required_role VARCHAR
		) RETURNS BOOLEAN AS $$
	DECLARE
		v_user_role VARCHAR;
		v_role_level INT;
		v_required_level INT;
	BEGIN
		SELECT role INTO v_user_role
		FROM app_permissions
		WHERE user_id = p_user_id
		  AND organization_id = p_organization_id
		  AND app_id = p_app_id;

		IF v_user_role IS NULL THEN
			SELECT role INTO v_user_role
			FROM app_permissions
			WHERE user_id = p_user_id
			  AND organization_id = p_organization_id
			  AND app_id = '` + globalAppID + `';
		END IF;

		IF v_user_role IS NULL THEN
			RETURN FALSE;
		END IF;

		v_user_role := LOWER(v_user_role);
		v_required_level := CASE LOWER(p_required_role)
			WHEN 'viewer' THEN 1
			WHEN 'member' THEN 2
			WHEN 'admin' THEN 3
			ELSE 999
		END;
		v_role_level := CASE v_user_role
			WHEN 'viewer' THEN 1
			WHEN 'member' THEN 2
			WHEN 'admin' THEN 3
			ELSE 0
		END;

		RETURN v_role_level >= v_required_level;
	END;
	$$ LANGUAGE plpgsql;`,
		`CREATE OR REPLACE FUNCTION get_or_create_local_user(
			p_citizenauth_uuid VARCHAR,
			p_email VARCHAR,
			p_name VARCHAR,
			p_organization_id UUID
		) RETURNS INTEGER AS $$
	DECLARE
		v_local_user_id INTEGER;
		v_username VARCHAR;
		v_base_username VARCHAR;
	BEGIN
		SELECT local_user_id INTO v_local_user_id
		FROM citizenauth_user_mapping
		WHERE citizenauth_user_id = p_citizenauth_uuid
		  AND organization_id = p_organization_id;

		IF v_local_user_id IS NOT NULL THEN
			UPDATE citizenauth_user_mapping
			SET last_login_at = CURRENT_TIMESTAMP,
				email = COALESCE(p_email, email),
				name = COALESCE(p_name, name),
				updated_at = CURRENT_TIMESTAMP
			WHERE citizenauth_user_id = p_citizenauth_uuid
			  AND organization_id = p_organization_id;

			RETURN v_local_user_id;
		END IF;

		IF p_email IS NOT NULL THEN
			SELECT local_user_id INTO v_local_user_id
			FROM citizenauth_user_mapping
			WHERE organization_id = p_organization_id
			  AND LOWER(email) = LOWER(p_email)
			ORDER BY last_login_at DESC NULLS LAST
			LIMIT 1;
		END IF;

		IF v_local_user_id IS NULL AND p_email IS NOT NULL THEN
			SELECT id INTO v_local_user_id
			FROM users
			WHERE LOWER(email) = LOWER(p_email)
			LIMIT 1;
		END IF;

		IF v_local_user_id IS NULL THEN
			IF p_email IS NULL OR p_email = '' THEN
				v_base_username := 'citizenauth_user';
			ELSE
				v_base_username := SPLIT_PART(p_email, '@', 1);
				IF v_base_username IS NULL OR v_base_username = '' THEN
					v_base_username := 'citizenauth_user';
				END IF;
			END IF;

			v_username := v_base_username;

			WHILE EXISTS (SELECT 1 FROM users WHERE username = v_username) LOOP
				v_username := v_base_username || '_' || FLOOR(RANDOM() * 1000)::TEXT;
			END LOOP;

			INSERT INTO users (username, password, email)
			VALUES (v_username, 'CITIZENAUTH_SSO', p_email)
			RETURNING id INTO v_local_user_id;
		ELSE
			IF p_email IS NOT NULL THEN
				UPDATE users
				SET email = p_email,
					updated_at = CURRENT_TIMESTAMP
				WHERE id = v_local_user_id
				  AND (email IS DISTINCT FROM p_email);
			END IF;
		END IF;

		INSERT INTO citizenauth_user_mapping (
			citizenauth_user_id,
			local_user_id,
			organization_id,
			email,
			name,
			last_login_at,
			updated_at
		) VALUES (
			p_citizenauth_uuid,
			v_local_user_id,
			p_organization_id,
			p_email,
			p_name,
			CURRENT_TIMESTAMP,
			CURRENT_TIMESTAMP
		)
		ON CONFLICT (citizenauth_user_id, organization_id)
		DO UPDATE SET
			local_user_id = EXCLUDED.local_user_id,
			email = COALESCE(EXCLUDED.email, citizenauth_user_mapping.email),
			name = COALESCE(EXCLUDED.name, citizenauth_user_mapping.name),
			last_login_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP;

		RETURN v_local_user_id;
	END;
	$$ LANGUAGE plpgsql;`,
		`CREATE OR REPLACE FUNCTION grant_app_permission(
			p_user_id VARCHAR,
			p_organization_id UUID,
			p_app_id VARCHAR,
			p_role VARCHAR,
			p_granted_by VARCHAR
		) RETURNS VOID AS $$
	BEGIN
		INSERT INTO app_permissions (
			user_id,
			organization_id,
			app_id,
			role,
			granted_by
		) VALUES (
			p_user_id,
			p_organization_id,
			p_app_id,
			p_role,
			p_granted_by
		)
		ON CONFLICT (user_id, app_id, organization_id)
		DO UPDATE SET
			role = EXCLUDED.role,
			granted_by = EXCLUDED.granted_by,
			updated_at = NOW();
	END;
	$$ LANGUAGE plpgsql;`,
	}

	for _, stmt := range statements {
		if _, err := database.DB.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func collectInstanceDomains(baseDomain string) ([]string, error) {
	domainSet := map[string]struct{}{}
	add := func(domain string) {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			return
		}
		domainSet[domain] = struct{}{}
	}

	add(baseDomain)

	database.ConnectDB()
	defer database.CloseDB()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := database.DB.Query(ctx, `SELECT app_name FROM app_deployments WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("load app deployments: %w", err)
	}
	for rows.Next() {
		var appName string
		if err := rows.Scan(&appName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan app name: %w", err)
		}
		appName = strings.TrimSpace(appName)
		if appName == "" || baseDomain == "" {
			continue
		}
		add(fmt.Sprintf("%s.%s", appName, baseDomain))
	}
	rows.Close()

	rows, err = database.DB.Query(ctx, `SELECT domain FROM app_deployments WHERE domain IS NOT NULL AND domain <> ''`)
	if err == nil {
		for rows.Next() {
			var domain string
			if err := rows.Scan(&domain); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan deployment domain: %w", err)
			}
			add(domain)
		}
		rows.Close()
	} else {
		return nil, fmt.Errorf("load deployment domains: %w", err)
	}

	rows, err = database.DB.Query(ctx, `SELECT domain FROM app_custom_domains WHERE is_active = true`)
	if err == nil {
		for rows.Next() {
			var domain string
			if err := rows.Scan(&domain); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan custom domain: %w", err)
			}
			add(domain)
		}
		rows.Close()
	} else {
		return nil, fmt.Errorf("load custom domains: %w", err)
	}

	var domains []string
	for domain := range domainSet {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains, nil
}

func syncInstanceDomains(bundle *BootstrapBundle) error {
	domains, err := collectInstanceDomains(bundle.Domain)
	if err != nil {
		return err
	}
	if len(domains) == 0 {
		return nil
	}

	payload := map[string]interface{}{
		"domains": domains,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal domain payload: %w", err)
	}

	updateURL := fmt.Sprintf("%s/api/v1/servers/%s/domains", baseURL(bundle.HandshakeURL), bundle.InstanceID)
	req, err := http.NewRequest(http.MethodPost, updateURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build domain sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", bundle.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sync domains request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("citizenauth domain sync failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func readEnvValue(path, key string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("env file path empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	prefix := key + "="
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):]), nil
		}
	}
	return "", fmt.Errorf("%s not found in %s", key, path)
}

func updateEnvFile(envFile string, bundle *BootstrapBundle) error {
	// Try multiple path patterns for both host and container environments
	candidates := []string{
		envFile,
		"./docker/.env",                      // From project root
		"docker/.env",                        // From project root (alternative)
		"../docker/.env",                     // From backend/
		"../../docker/.env",                  // From backend/cmd/
		"/app/../docker/.env",                // From container /app -> host docker/
		"/root/projects/citizen/docker/.env", // Absolute path (host)
	}
	path, err := findExistingPath(candidates)
	if err != nil {
		fmt.Printf("⚠️ env update skipped: %v\n", err)
		fmt.Printf("💡 Tip: Run this command from the Citizen project root (/root/projects/citizen/)\n")
		fmt.Printf("   Or manually update docker/.env with CITIZENAUTH_URL and CITIZENAUTH_JWKS_URL\n")
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	base := baseURL(bundle.HandshakeURL)
	jwks := strings.TrimRight(base, "/") + "/api/v1/auth/jwks.json"

	keysToRemove := map[string]struct{}{
		"CITIZENAUTH_API_KEY":        {},
		"CITIZENAUTH_API_KEY_HASH":   {},
		"CITIZENAUTH_WEBHOOK_SECRET": {},
	}

	replacements := map[string]string{
		"CITIZENAUTH_URL":      fmt.Sprintf("CITIZENAUTH_URL=%s", base),
		"CITIZENAUTH_JWKS_URL": fmt.Sprintf("CITIZENAUTH_JWKS_URL=%s", jwks),
	}

	found := make(map[string]bool)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for key := range keysToRemove {
			if strings.HasPrefix(trimmed, key+"=") {
				lines[i] = "# " + line + " (managed by instance import)"
			}
		}
		for key, value := range replacements {
			if strings.HasPrefix(trimmed, key+"=") {
				lines[i] = value
				found[key] = true
			}
		}
	}

	for key, value := range replacements {
		if !found[key] {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			lines = append(lines, value)
		}
	}

	output := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(output), 0644); err != nil {
		return err
	}

	return nil
}

func restartService(composeFile, envFile, service string) error {
	// Try multiple path patterns for both host and container environments
	composePath, err := findExistingPath([]string{
		composeFile,
		"./docker/docker-compose.prod.yml",
		"docker/docker-compose.prod.yml",
		"../docker/docker-compose.prod.yml",
		"../../docker/docker-compose.prod.yml",
		"/app/../docker/docker-compose.prod.yml",                // From container /app -> host docker/
		"/root/projects/citizen/docker/docker-compose.prod.yml", // Absolute path (host)
	})
	if err != nil {
		fmt.Printf("⚠️ restart skipped: %v\n", err)
		fmt.Printf("💡 Tip: If running inside container, restart manually from host:\n")
		fmt.Printf("   docker compose -f /root/projects/citizen/docker/docker-compose.prod.yml restart api\n")
		return nil
	}

	envPath, _ := findExistingPath([]string{
		envFile,
		"./docker/.env",
		"docker/.env",
		"../docker/.env",
		"../../docker/.env",
		"/app/../docker/.env",
		"/root/projects/citizen/docker/.env",
	})

	serviceName := service
	if serviceName == "" {
		serviceName = "api"
	}

	var envArgs []string
	if envPath != "" {
		envArgs = []string{"--env-file", envPath}
	}

	composeArgs := append([]string{"-f", composePath}, envArgs...)
	composeArgs = append(composeArgs, "restart", serviceName)

	fmt.Printf("🔄 Restarting Docker service: %s\n", serviceName)

	// Try docker compose (new syntax)
	if _, err := exec.LookPath("docker"); err == nil {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			cmd := exec.Command("docker", append([]string{"compose"}, composeArgs...)...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("docker compose restart failed: %w", err)
			}
			fmt.Printf("✅ Successfully restarted %s via docker compose\n", serviceName)
			return nil
		}
	}

	// Try docker-compose (legacy syntax)
	if path, err := exec.LookPath("docker-compose"); err == nil {
		args := append([]string{"-f", composePath}, envArgs...)
		args = append(args, "restart", serviceName)
		cmd := exec.Command(path, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker-compose restart failed: %w", err)
		}
		fmt.Printf("✅ Successfully restarted %s via docker-compose\n", serviceName)
		return nil
	}

	fmt.Println("⚠️ restart skipped: docker compose not available on this host")
	fmt.Println("   Please manually restart the Citizen API container:")
	fmt.Printf("   docker compose -f %s restart %s\n", composePath, serviceName)
	return nil
}

func computeChallenge(apiKey, nonce string) string {
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomHex(size int) string {
	b := make([]byte, size)
	if _, err := crand.Read(b); err != nil {
		rand.Seed(time.Now().UnixNano())
		for i := range b {
			b[i] = byte(rand.Intn(256))
		}
	}
	return hex.EncodeToString(b)
}

func baseURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return raw
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "❌ %v\n", err)
	os.Exit(1)
}

func findExistingPath(candidates []string) (string, error) {
	var tried []string
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			tried = append(tried, candidate)
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
		tried = append(tried, abs)
	}
	if len(tried) == 0 {
		return "", fmt.Errorf("no candidate paths provided")
	}
	return "", fmt.Errorf("looked in: %s", strings.Join(tried, ", "))
}
