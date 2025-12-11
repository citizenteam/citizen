package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const httpChallengeFilePath = "/app/docker/config/http_challenges.json"

type httpChallengeEntry struct {
	Host      string `json:"host"`
	Path      string `json:"path"`
	Body      string `json:"body"`
	UpdatedAt string `json:"updated_at"`
}

func ensureChallengeDir() error {
	dir := filepath.Dir(httpChallengeFilePath)
	return os.MkdirAll(dir, 0o750)
}

func loadChallengeEntries() ([]httpChallengeEntry, error) {
	if err := ensureChallengeDir(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(httpChallengeFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []httpChallengeEntry{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []httpChallengeEntry{}, nil
	}
	var entries []httpChallengeEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func saveChallengeEntries(entries []httpChallengeEntry) error {
	if err := ensureChallengeDir(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tempPath := httpChallengeFilePath + ".tmp"
	if err := os.WriteFile(tempPath, append(payload, '\n'), 0o640); err != nil {
		return err
	}
	return os.Rename(tempPath, httpChallengeFilePath)
}

// AddHTTPChallengeEntry persists/updates a challenge entry for Traefik.
func AddHTTPChallengeEntry(host, path, body string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	entries, err := loadChallengeEntries()
	if err != nil {
		return err
	}

	updated := false
	for i := range entries {
		if entries[i].Host == host && entries[i].Path == path {
			entries[i].Body = body
			entries[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = true
			break
		}
	}
	if !updated {
		entries = append(entries, httpChallengeEntry{
			Host:      host,
			Path:      path,
			Body:      body,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Host == entries[j].Host {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Host < entries[j].Host
	})

	return saveChallengeEntries(entries)
}

// RemoveHTTPChallengeEntry deletes a challenge entry by host/path.
func RemoveHTTPChallengeEntry(host, path string) (bool, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false, fmt.Errorf("host is required")
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	entries, err := loadChallengeEntries()
	if err != nil {
		return false, err
	}

	filtered := make([]httpChallengeEntry, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if entry.Host == host && entry.Path == path {
			removed = true
			continue
		}
		filtered = append(filtered, entry)
	}

	if !removed {
		return false, nil
	}

	if err := saveChallengeEntries(filtered); err != nil {
		return false, err
	}

	return true, nil
}
