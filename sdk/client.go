package sdk

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
)

const (
	defaultDatabaseName = "openplanner.db"
)

type Options struct {
	// DatabasePath overrides the default SQLite path.
	// When empty, OpenPlanner stores data under ${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db.
	DatabasePath string
}

type Client struct {
	service *service.Service
	closeFn func() error
}

// OpenLocal opens the direct local OpenPlanner runtime.
func OpenLocal(options Options) (*Client, error) {
	databasePath, err := resolveDatabasePath(options.DatabasePath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}

	repository, err := store.Open(databasePath)
	if err != nil {
		return nil, err
	}

	return &Client{
		service: service.New(repository),
		closeFn: repository.Close,
	}, nil
}

// DefaultDataDir returns the default XDG-style data directory for OpenPlanner.
func DefaultDataDir() (string, error) {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "openplanner"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("resolve user home: empty value")
	}

	return filepath.Join(home, ".local", "share", "openplanner"), nil
}

// DefaultDatabasePath returns the default SQLite path for OpenPlanner.
func DefaultDatabasePath() (string, error) {
	dataDir, err := DefaultDataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dataDir, defaultDatabaseName), nil
}

func resolveDatabasePath(databasePath string) (string, error) {
	if databasePath != "" {
		return databasePath, nil
	}

	return DefaultDatabasePath()
}

func (client *Client) Close() error {
	if client == nil || client.closeFn == nil {
		return nil
	}

	return client.closeFn()
}
