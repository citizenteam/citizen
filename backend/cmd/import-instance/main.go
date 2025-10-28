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
	"strings"
	"time"

	"backend/database"
	"backend/utils"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

const globalAppID = "__all__"

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
		exitWithError(fmt.Errorf("citizenauth handshake: %w", err))
	}

	if err := storeSecretsLocally(bundle, envFile); err != nil {
		exitWithError(fmt.Errorf("store secrets in citizen: %w", err))
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
		return fmt.Errorf("citizenauth handshake failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	return nil
}

func storeSecretsLocally(bundle *BootstrapBundle, envFile string) error {
	// Attempt to load ENV from the provided path first, then fall back to common defaults.
	if envFile != "" {
		_ = godotenv.Overload(envFile)
	}
	_ = godotenv.Overload("./docker/.env")
	_ = godotenv.Overload("../docker/.env")

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
	candidates := []string{envFile, "./docker/.env", "../docker/.env", "../../docker/.env"}
	path, err := findExistingPath(candidates)
	if err != nil {
		fmt.Printf("⚠️ env update skipped: %v\n", err)
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
	composePath, err := findExistingPath([]string{
		composeFile,
		"./docker/docker-compose.prod.yml",
		"../docker/docker-compose.prod.yml",
		"../../docker/docker-compose.prod.yml",
	})
	if err != nil {
		fmt.Printf("⚠️ restart skipped: %v\n", err)
		return nil
	}

	envPath, _ := findExistingPath([]string{
		envFile,
		"./docker/.env",
		"../docker/.env",
		"../../docker/.env",
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

	if _, err := exec.LookPath("docker"); err == nil {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			cmd := exec.Command("docker", append([]string{"compose"}, composeArgs...)...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
	}

	if path, err := exec.LookPath("docker-compose"); err == nil {
		args := append([]string{"-f", composePath}, envArgs...)
		args = append(args, "restart", serviceName)
		cmd := exec.Command(path, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	fmt.Println("⚠️ restart skipped: docker compose not available on this host")
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
