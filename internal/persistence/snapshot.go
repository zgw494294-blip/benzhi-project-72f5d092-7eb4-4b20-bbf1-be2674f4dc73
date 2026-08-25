package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func writeSnapshot(path string, state snapshot) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*.tmp")
	if err != nil {
		return err
	}
	tempName := temporary.Name()
	cleanup := func() { temporary.Close(); os.Remove(tempName) }
	if _, err = temporary.Write(data); err != nil {
		cleanup()
		return err
	}
	if err = temporary.Sync(); err != nil {
		cleanup()
		return err
	}
	if err = temporary.Close(); err != nil {
		os.Remove(tempName)
		return err
	}
	if err = os.Rename(tempName, path); err != nil {
		os.Remove(tempName)
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func readSnapshot(path string) (*snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state snapshot
	if err = json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("快照损坏: %w", err)
	}
	if state.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("不支持的快照 schemaVersion: %d", state.SchemaVersion)
	}
	return &state, nil
}
