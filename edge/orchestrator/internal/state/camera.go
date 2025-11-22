package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SaveCamera saves or updates a camera in the database
func (m *Manager) SaveCamera(ctx context.Context, cam CameraState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `
		INSERT INTO cameras (id, name, rtsp_url, enabled, last_seen, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			rtsp_url = excluded.rtsp_url,
			enabled = excluded.enabled,
			last_seen = excluded.last_seen,
			updated_at = excluded.updated_at
	`

	var lastSeen interface{}
	if cam.LastSeen != nil {
		lastSeen = *cam.LastSeen
	}

	_, err := m.db.GetDB().ExecContext(ctx, query,
		cam.ID, cam.Name, cam.RTSPURL, cam.Enabled, lastSeen, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to save camera: %w", err)
	}

	return nil
}

// GetCamera retrieves a camera by ID
func (m *Manager) GetCamera(ctx context.Context, cameraID string) (*CameraState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, name, rtsp_url, enabled, last_seen FROM cameras WHERE id = ?`
	
	var cam CameraState
	var lastSeen sql.NullTime
	err := m.db.GetDB().QueryRowContext(ctx, query, cameraID).Scan(
		&cam.ID, &cam.Name, &cam.RTSPURL, &cam.Enabled, &lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get camera: %w", err)
	}

	if lastSeen.Valid {
		cam.LastSeen = &lastSeen.Time
	}

	return &cam, nil
}

// UpdateCameraLastSeen updates the last seen timestamp for a camera
func (m *Manager) UpdateCameraLastSeen(ctx context.Context, cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `UPDATE cameras SET last_seen = ?, updated_at = ? WHERE id = ?`
	_, err := m.db.GetDB().ExecContext(ctx, query, time.Now(), time.Now(), cameraID)
	if err != nil {
		return fmt.Errorf("failed to update camera last seen: %w", err)
	}

	return nil
}

// ListCameras lists all cameras
func (m *Manager) ListCameras(ctx context.Context, enabledOnly bool) ([]CameraState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, name, rtsp_url, enabled, last_seen FROM cameras`
	if enabledOnly {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY name`

	rows, err := m.db.GetDB().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list cameras: %w", err)
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

// DeleteCamera deletes a camera
func (m *Manager) DeleteCamera(ctx context.Context, cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	query := `DELETE FROM cameras WHERE id = ?`
	_, err := m.db.GetDB().ExecContext(ctx, query, cameraID)
	if err != nil {
		return fmt.Errorf("failed to delete camera: %w", err)
	}

	return nil
}

