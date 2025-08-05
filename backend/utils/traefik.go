package utils

import (
	"fmt"
	"os"
	"time"
)

func ReloadTraefik() error {
	// Create a signal file that dokku-traefik-watcher will detect
	signalPath := "/tmp/traefik-reload-signal"
	
	// Create or touch the signal file
	file, err := os.Create(signalPath)
	if err != nil {
		return fmt.Errorf("failed to create signal file: %v", err)
	}
	defer file.Close()
	
	// Write timestamp to the file
	_, err = file.WriteString(time.Now().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to write to signal file: %v", err)
	}
	
	return nil
}