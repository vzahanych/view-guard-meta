package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

// Manager manages system state persistence and recovery
type Manager struct {
	db     *Database
	logger *logger.Logger
	mu     sync.RWMutex
}

// NewManager creates a new state manager
func NewManager(cfg *config.Config, log *logger.Logger) (*Manager, error) {
	// Determine database path
	dbPath := filepath.Join(cfg.Edge.Orchestrator.DataDir, "db", "edge.db")
	
	// Ensure database directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Create database
	db, err := NewDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return &Manager{
		db:     db,
		logger: log,
	}, nil
}

// Close closes the state manager and database
func (m *Manager) Close() error {
	return m.db.Close()
}

// GetDB returns the database connection
func (m *Manager) GetDB() *sql.DB {
	return m.db.GetDB()
}

// SaveSystemState saves a system state value
func (m *Manager) SaveSystemState(ctx context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
		INSERT INTO system_state (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`

	_, err := m.db.GetDB().ExecContext(ctx, query, key, value, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save system state: %w", err)
	}

	return nil
}

// GetSystemState retrieves a system state value
func (m *Manager) GetSystemState(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var value string
	query := `SELECT value FROM system_state WHERE key = ?`
	err := m.db.GetDB().QueryRowContext(ctx, query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get system state: %w", err)
	}

	return value, nil
}

// RecoverState recovers system state on startup
func (m *Manager) RecoverState(ctx context.Context) (*RecoveredState, error) {
	m.logger.Info("Recovering system state")

	recovered := &RecoveredState{
		Cameras: make([]CameraState, 0),
		Events:  make([]EventState, 0),
	}

	// Recover cameras
	cameras, err := m.recoverCameras(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to recover cameras: %w", err)
	}
	recovered.Cameras = cameras

	// Recover pending events
	events, err := m.recoverPendingEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to recover events: %w", err)
	}
	recovered.Events = events

	// Recover system state
	systemState, err := m.recoverSystemState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to recover system state: %w", err)
	}
	recovered.SystemState = systemState

	m.logger.Info("State recovery complete",
		"cameras", len(recovered.Cameras),
		"pending_events", len(recovered.Events),
	)

	return recovered, nil
}

// RecoveredState represents the state recovered on startup
type RecoveredState struct {
	Cameras    []CameraState
	Events     []EventState
	SystemState map[string]string
}

// CameraState represents a camera's persisted state
type CameraState struct {
	ID       string
	Name     string
	RTSPURL  string
	Enabled  bool
	LastSeen *time.Time
}

// EventState represents an event's persisted state
type EventState struct {
	ID          string
	CameraID    string
	EventType   string
	Timestamp   time.Time
	Metadata    map[string]interface{}
	ClipPath    string
	SnapshotPath string
	Transmitted bool
}

// recoverCameras recovers camera states
func (m *Manager) recoverCameras(ctx context.Context) ([]CameraState, error) {
	query := `
		SELECT id, name, rtsp_url, enabled, last_seen
		FROM cameras
		WHERE enabled = 1
		ORDER BY name
	`

	rows, err := m.db.GetDB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cameras []CameraState
	for rows.Next() {
		var cam CameraState
		var lastSeen sql.NullTime
		if err := rows.Scan(&cam.ID, &cam.Name, &cam.RTSPURL, &cam.Enabled, &lastSeen); err != nil {
			return nil, err
		}
		if lastSeen.Valid {
			cam.LastSeen = &lastSeen.Time
		}
		cameras = append(cameras, cam)
	}

	return cameras, rows.Err()
}

// recoverPendingEvents recovers pending events (not yet transmitted)
func (m *Manager) recoverPendingEvents(ctx context.Context) ([]EventState, error) {
	query := `
		SELECT id, camera_id, event_type, timestamp, metadata, clip_path, snapshot_path, transmitted
		FROM events
		WHERE transmitted = 0
		ORDER BY timestamp ASC
		LIMIT 1000
	`

	rows, err := m.db.GetDB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventState
	for rows.Next() {
		var event EventState
		var metadataJSON sql.NullString
		if err := rows.Scan(
			&event.ID, &event.CameraID, &event.EventType, &event.Timestamp,
			&metadataJSON, &event.ClipPath, &event.SnapshotPath, &event.Transmitted,
		); err != nil {
			return nil, err
		}

		// Parse metadata JSON
		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &event.Metadata); err != nil {
				m.logger.Warn("Failed to parse event metadata", "event_id", event.ID, "error", err)
				event.Metadata = make(map[string]interface{})
			}
		} else {
			event.Metadata = make(map[string]interface{})
		}

		events = append(events, event)
	}

	return events, rows.Err()
}

// recoverSystemState recovers system state
func (m *Manager) recoverSystemState(ctx context.Context) (map[string]string, error) {
	query := `SELECT key, value FROM system_state`
	rows, err := m.db.GetDB().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	state := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		state[key] = value
	}

	return state, rows.Err()
}

// SyncState synchronizes state (placeholder for future implementation)
func (m *Manager) SyncState(ctx context.Context) error {
	// This would be used for synchronizing state with KVM VM or other services
	// For now, it's a placeholder
	m.logger.Debug("State synchronization called (not yet implemented)")
	return nil
}

