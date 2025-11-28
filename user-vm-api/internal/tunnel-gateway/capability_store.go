package tunnelgateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vzahanych/view-guard-meta/proto/go/generated/edge"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/database"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/service"
)

// TrainingEligibilityStatus represents the training readiness state of a camera
type TrainingEligibilityStatus string

const (
	TrainingEligibilityNeedsSnapshots    TrainingEligibilityStatus = "needs_snapshots"
	TrainingEligibilityReadyForTraining TrainingEligibilityStatus = "ready_for_training"
	TrainingEligibilityTrainingInProgress TrainingEligibilityStatus = "training_in_progress"
)

// CapabilityStore persists camera capability/dataset readiness data reported by Edge
type CapabilityStore struct {
	db       *database.DB
	eventBus *service.EventBus
}

// NewCapabilityStore creates a new capability store
func NewCapabilityStore(db *database.DB) *CapabilityStore {
	return &CapabilityStore{db: db}
}

// SetEventBus sets the event bus for publishing state transition events
func (s *CapabilityStore) SetEventBus(bus *service.EventBus) {
	s.eventBus = bus
}

// UpsertCapabilities stores capability records for an edge and detects state transitions
func (s *CapabilityStore) UpsertCapabilities(ctx context.Context, edgeID string, cameras []*edge.CameraCapability, syncedAt time.Time) error {
	if len(cameras) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op if already committed

	// Get existing states to detect transitions
	existingStates := make(map[string]TrainingEligibilityStatus)
	const getExistingSQL = `SELECT camera_id, training_eligibility_status FROM edge_camera_status WHERE edge_id = ?`
	rows, err := tx.QueryContext(ctx, getExistingSQL, edgeID)
	if err == nil {
		for rows.Next() {
			var cameraID string
			var status string
			if err := rows.Scan(&cameraID, &status); err == nil {
				existingStates[cameraID] = TrainingEligibilityStatus(status)
			}
		}
		rows.Close()
	}

	const upsertSQL = `
	INSERT INTO edge_camera_status (
		edge_id,
		camera_id,
		camera_name,
		camera_type,
		camera_status,
		enabled,
		label_counts,
		labeled_snapshot_count,
		required_snapshot_count,
		snapshot_required,
		training_eligibility_status,
		synced_at,
		updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(edge_id, camera_id) DO UPDATE SET
		camera_name = excluded.camera_name,
		camera_type = excluded.camera_type,
		camera_status = excluded.camera_status,
		enabled = excluded.enabled,
		label_counts = excluded.label_counts,
		labeled_snapshot_count = excluded.labeled_snapshot_count,
		required_snapshot_count = excluded.required_snapshot_count,
		snapshot_required = excluded.snapshot_required,
		training_eligibility_status = excluded.training_eligibility_status,
		synced_at = excluded.synced_at,
		updated_at = excluded.updated_at
	`

	now := time.Now().Unix()
	var stateTransitions []StateTransition

	for _, cam := range cameras {
		labelCountsJSON, err := json.Marshal(cam.LabelCounts)
		if err != nil {
			return fmt.Errorf("failed to marshal label_counts for camera %s: %w", cam.CameraId, err)
		}

		// Determine training eligibility status
		newStatus := s.determineTrainingEligibility(cam)
		oldStatus, existed := existingStates[cam.CameraId]

		// Detect state transition
		if existed && oldStatus != newStatus {
			stateTransitions = append(stateTransitions, StateTransition{
				EdgeID:      edgeID,
				CameraID:    cam.CameraId,
				CameraName:  cam.Name,
				OldStatus:   oldStatus,
				NewStatus:   newStatus,
			})
		}

		if _, err := tx.ExecContext(ctx, upsertSQL,
			edgeID,
			cam.CameraId,
			cam.Name,
			cam.Type,
			cam.Status,
			boolToInt(cam.Enabled),
			string(labelCountsJSON),
			cam.LabeledSnapshotCount,
			cam.RequiredSnapshotCount,
			boolToInt(cam.SnapshotRequired),
			string(newStatus),
			syncedAt.Unix(),
			now,
		); err != nil {
			return fmt.Errorf("failed to upsert camera status for %s: %w", cam.CameraId, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit capability transaction: %w", err)
	}

	// Publish state transition events
	s.publishStateTransitions(stateTransitions)

	return nil
}

// determineTrainingEligibility determines the training eligibility status based on camera capability
func (s *CapabilityStore) determineTrainingEligibility(cam *edge.CameraCapability) TrainingEligibilityStatus {
	if cam.SnapshotRequired {
		return TrainingEligibilityNeedsSnapshots
	}
	// If snapshot_required is false, camera has enough snapshots and is ready for training
	// Note: training_in_progress status would be set separately when training actually starts
	return TrainingEligibilityReadyForTraining
}

// StateTransition represents a camera training eligibility state change
type StateTransition struct {
	EdgeID     string
	CameraID   string
	CameraName string
	OldStatus  TrainingEligibilityStatus
	NewStatus  TrainingEligibilityStatus
}

// publishStateTransitions publishes events for state transitions
func (s *CapabilityStore) publishStateTransitions(transitions []StateTransition) {
	if s.eventBus == nil || len(transitions) == 0 {
		return
	}

	for _, transition := range transitions {
		eventTypeStr := s.getEventTypeForTransition(transition.OldStatus, transition.NewStatus)
		if eventTypeStr != "" {
			s.eventBus.Publish(service.Event{
				Type:      service.EventType(eventTypeStr),
				Timestamp: time.Now().Unix(),
				Data: map[string]interface{}{
					"edge_id":     transition.EdgeID,
					"camera_id":   transition.CameraID,
					"camera_name": transition.CameraName,
					"old_status":  string(transition.OldStatus),
					"new_status":  string(transition.NewStatus),
				},
			})
		}
	}
}

// getEventTypeForTransition returns the event type for a state transition
func (s *CapabilityStore) getEventTypeForTransition(oldStatus, newStatus TrainingEligibilityStatus) string {
	switch {
	case newStatus == TrainingEligibilityReadyForTraining && oldStatus == TrainingEligibilityNeedsSnapshots:
		return "camera.ready_for_training"
	case newStatus == TrainingEligibilityTrainingInProgress:
		return "camera.training_started"
	case newStatus == TrainingEligibilityNeedsSnapshots && oldStatus == TrainingEligibilityReadyForTraining:
		return "camera.needs_snapshots"
	default:
		return ""
	}
}

// ListCameraStatuses lists stored statuses for an edge
func (s *CapabilityStore) ListCameraStatuses(ctx context.Context, edgeID string) ([]EdgeCameraStatus, error) {
	const query = `
	SELECT camera_id, camera_name, camera_type, camera_status, enabled,
	       label_counts, labeled_snapshot_count, required_snapshot_count,
	       snapshot_required, training_eligibility_status, synced_at, updated_at
	FROM edge_camera_status
	WHERE edge_id = ?
	ORDER BY camera_name
	`

	rows, err := s.db.QueryContext(ctx, query, edgeID)
	if err != nil {
		return nil, fmt.Errorf("failed to query camera statuses: %w", err)
	}
	defer rows.Close()

	var statuses []EdgeCameraStatus
	for rows.Next() {
		var status EdgeCameraStatus
		var enabledInt, snapshotRequiredInt int
		var labelCounts sql.NullString
		var trainingEligibilityStatus string
		var syncedAt, updatedAt int64

		if err := rows.Scan(
			&status.CameraID,
			&status.Name,
			&status.Type,
			&status.Status,
			&enabledInt,
			&labelCounts,
			&status.LabeledSnapshotCount,
			&status.RequiredSnapshotCount,
			&snapshotRequiredInt,
			&trainingEligibilityStatus,
			&syncedAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan camera status: %w", err)
		}

		status.Enabled = enabledInt == 1
		status.SnapshotRequired = snapshotRequiredInt == 1
		status.TrainingEligibilityStatus = TrainingEligibilityStatus(trainingEligibilityStatus)
		status.SyncedAt = time.Unix(syncedAt, 0)
		status.UpdatedAt = time.Unix(updatedAt, 0)

		if labelCounts.Valid && labelCounts.String != "" {
			_ = json.Unmarshal([]byte(labelCounts.String), &status.LabelCounts)
		}

		statuses = append(statuses, status)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating camera statuses: %w", err)
	}

	return statuses, nil
}

// EdgeCameraStatus represents stored capability info
type EdgeCameraStatus struct {
	CameraID                  string
	Name                      string
	Type                      string
	Status                    string
	Enabled                   bool
	LabelCounts               map[string]uint32
	LabeledSnapshotCount      uint32
	RequiredSnapshotCount      uint32
	SnapshotRequired          bool
	TrainingEligibilityStatus  TrainingEligibilityStatus
	SyncedAt                  time.Time
	UpdatedAt                 time.Time
}

// GetCameraStatus retrieves status for a specific camera
func (s *CapabilityStore) GetCameraStatus(ctx context.Context, edgeID, cameraID string) (*EdgeCameraStatus, error) {
	const query = `
	SELECT camera_id, camera_name, camera_type, camera_status, enabled,
	       label_counts, labeled_snapshot_count, required_snapshot_count,
	       snapshot_required, training_eligibility_status, synced_at, updated_at
	FROM edge_camera_status
	WHERE edge_id = ? AND camera_id = ?
	`

	var status EdgeCameraStatus
	var enabledInt, snapshotRequiredInt int
	var labelCounts sql.NullString
	var trainingEligibilityStatus string
	var syncedAt, updatedAt int64

	err := s.db.QueryRowContext(ctx, query, edgeID, cameraID).Scan(
		&status.CameraID,
		&status.Name,
		&status.Type,
		&status.Status,
		&enabledInt,
		&labelCounts,
		&status.LabeledSnapshotCount,
		&status.RequiredSnapshotCount,
		&snapshotRequiredInt,
		&trainingEligibilityStatus,
		&syncedAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("camera status not found: edge_id=%s, camera_id=%s", edgeID, cameraID)
		}
		return nil, fmt.Errorf("failed to query camera status: %w", err)
	}

	status.Enabled = enabledInt == 1
	status.SnapshotRequired = snapshotRequiredInt == 1
	status.TrainingEligibilityStatus = TrainingEligibilityStatus(trainingEligibilityStatus)
	status.SyncedAt = time.Unix(syncedAt, 0)
	status.UpdatedAt = time.Unix(updatedAt, 0)

	if labelCounts.Valid && labelCounts.String != "" {
		_ = json.Unmarshal([]byte(labelCounts.String), &status.LabelCounts)
	}

	return &status, nil
}

// ListCamerasReadyForTraining returns cameras that are ready for training
func (s *CapabilityStore) ListCamerasReadyForTraining(ctx context.Context, edgeID string) ([]EdgeCameraStatus, error) {
	const query = `
	SELECT camera_id, camera_name, camera_type, camera_status, enabled,
	       label_counts, labeled_snapshot_count, required_snapshot_count,
	       snapshot_required, training_eligibility_status, synced_at, updated_at
	FROM edge_camera_status
	WHERE edge_id = ? AND training_eligibility_status = ?
	ORDER BY camera_name
	`
	return s.listCamerasByEligibility(ctx, query, edgeID, TrainingEligibilityReadyForTraining)
}

// ListCamerasNeedingSnapshots returns cameras that need more snapshots
func (s *CapabilityStore) ListCamerasNeedingSnapshots(ctx context.Context, edgeID string) ([]EdgeCameraStatus, error) {
	const query = `
	SELECT camera_id, camera_name, camera_type, camera_status, enabled,
	       label_counts, labeled_snapshot_count, required_snapshot_count,
	       snapshot_required, training_eligibility_status, synced_at, updated_at
	FROM edge_camera_status
	WHERE edge_id = ? AND training_eligibility_status = ?
	ORDER BY camera_name
	`
	return s.listCamerasByEligibility(ctx, query, edgeID, TrainingEligibilityNeedsSnapshots)
}

// listCamerasByEligibility is a helper to query cameras by eligibility status
func (s *CapabilityStore) listCamerasByEligibility(ctx context.Context, query, edgeID string, status TrainingEligibilityStatus) ([]EdgeCameraStatus, error) {
	rows, err := s.db.QueryContext(ctx, query, edgeID, string(status))
	if err != nil {
		return nil, fmt.Errorf("failed to query cameras by eligibility: %w", err)
	}
	defer rows.Close()

	var statuses []EdgeCameraStatus
	for rows.Next() {
		var status EdgeCameraStatus
		var enabledInt, snapshotRequiredInt int
		var labelCounts sql.NullString
		var trainingEligibilityStatus string
		var syncedAt, updatedAt int64

		if err := rows.Scan(
			&status.CameraID,
			&status.Name,
			&status.Type,
			&status.Status,
			&enabledInt,
			&labelCounts,
			&status.LabeledSnapshotCount,
			&status.RequiredSnapshotCount,
			&snapshotRequiredInt,
			&trainingEligibilityStatus,
			&syncedAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan camera status: %w", err)
		}

		status.Enabled = enabledInt == 1
		status.SnapshotRequired = snapshotRequiredInt == 1
		status.TrainingEligibilityStatus = TrainingEligibilityStatus(trainingEligibilityStatus)
		status.SyncedAt = time.Unix(syncedAt, 0)
		status.UpdatedAt = time.Unix(updatedAt, 0)

		if labelCounts.Valid && labelCounts.String != "" {
			_ = json.Unmarshal([]byte(labelCounts.String), &status.LabelCounts)
		}

		statuses = append(statuses, status)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating camera statuses: %w", err)
	}

	return statuses, nil
}

// SetTrainingInProgress marks a camera as training in progress
func (s *CapabilityStore) SetTrainingInProgress(ctx context.Context, edgeID, cameraID string) error {
	const updateSQL = `
	UPDATE edge_camera_status
	SET training_eligibility_status = ?, updated_at = ?
	WHERE edge_id = ? AND camera_id = ?
	`

	now := time.Now().Unix()
	result, err := s.db.ExecContext(ctx, updateSQL, string(TrainingEligibilityTrainingInProgress), now, edgeID, cameraID)
	if err != nil {
		return fmt.Errorf("failed to update training status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("camera not found: edge_id=%s, camera_id=%s", edgeID, cameraID)
	}

	// Get old status and publish transition event
	oldStatus, err := s.GetCameraStatus(ctx, edgeID, cameraID)
	if err == nil && oldStatus.TrainingEligibilityStatus != TrainingEligibilityTrainingInProgress {
		if s.eventBus != nil {
			s.eventBus.Publish(service.Event{
				Type:      "camera.training_started",
				Timestamp: time.Now().Unix(),
				Data: map[string]interface{}{
					"edge_id":     edgeID,
					"camera_id":   cameraID,
					"camera_name": oldStatus.Name,
					"old_status":  string(oldStatus.TrainingEligibilityStatus),
					"new_status":  string(TrainingEligibilityTrainingInProgress),
				},
			})
		}
	}

	return nil
}

func boolToInt(val bool) int {
	if val {
		return 1
	}
	return 0
}

