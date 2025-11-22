package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SaveEvent saves an event to the database
func (m *Manager) SaveEvent(ctx context.Context, event EventState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialize metadata to JSON
	metadataJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO events (id, camera_id, event_type, timestamp, metadata, clip_path, snapshot_path, transmitted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			transmitted = excluded.transmitted,
			transmitted_at = CASE WHEN excluded.transmitted = 1 THEN CURRENT_TIMESTAMP ELSE transmitted_at END
	`

	_, err = m.db.GetDB().ExecContext(ctx, query,
		event.ID, event.CameraID, event.EventType, event.Timestamp,
		string(metadataJSON), event.ClipPath, event.SnapshotPath, event.Transmitted,
	)
	if err != nil {
		return fmt.Errorf("failed to save event: %w", err)
	}

	// If not transmitted, add to queue
	if !event.Transmitted {
		queueQuery := `
			INSERT OR IGNORE INTO event_queue (id, event_id, priority, retry_count)
			VALUES (?, ?, 0, 0)
		`
		_, err = m.db.GetDB().ExecContext(ctx, queueQuery, "queue_"+event.ID, event.ID)
		if err != nil {
			return fmt.Errorf("failed to add event to queue: %w", err)
		}
	}

	return nil
}

// MarkEventTransmitted marks an event as transmitted
func (m *Manager) MarkEventTransmitted(ctx context.Context, eventID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tx, err := m.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update event
	_, err = tx.ExecContext(ctx,
		`UPDATE events SET transmitted = 1, transmitted_at = ? WHERE id = ?`,
		time.Now(), eventID,
	)
	if err != nil {
		return fmt.Errorf("failed to mark event as transmitted: %w", err)
	}

	// Remove from queue
	_, err = tx.ExecContext(ctx, `DELETE FROM event_queue WHERE event_id = ?`, eventID)
	if err != nil {
		return fmt.Errorf("failed to remove event from queue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetPendingEvents retrieves pending events from the queue
func (m *Manager) GetPendingEvents(ctx context.Context, limit int) ([]EventState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT e.id, e.camera_id, e.event_type, e.timestamp, e.metadata, e.clip_path, e.snapshot_path, e.transmitted
		FROM events e
		INNER JOIN event_queue eq ON e.id = eq.event_id
		WHERE e.transmitted = 0
		ORDER BY eq.priority DESC, eq.created_at ASC
		LIMIT ?
	`

	rows, err := m.db.GetDB().QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending events: %w", err)
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
				event.Metadata = make(map[string]interface{})
			}
		} else {
			event.Metadata = make(map[string]interface{})
		}

		events = append(events, event)
	}

	return events, rows.Err()
}

// CleanupOldEvents removes old transmitted events (optional cleanup)
func (m *Manager) CleanupOldEvents(ctx context.Context, olderThan time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	query := `DELETE FROM events WHERE transmitted = 1 AND transmitted_at < ?`

	result, err := m.db.GetDB().ExecContext(ctx, query, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old events: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	m.logger.Debug("Cleaned up old events", "count", rowsAffected)

	return nil
}

