package types

import (
	"time"

	"github.com/google/uuid"
	aigateway "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/ai-gateway/types"
)

// PendingSnapshotRequest represents a pending snapshot capture request from VM
type PendingSnapshotRequest struct {
	CameraID    string    `json:"camera_id"`
	Label       string    `json:"label"`
	CustomLabel string    `json:"custom_label,omitempty"`
	Count       int32     `json:"count"`
	RequestedAt time.Time `json:"requested_at"`
}

// SecurityEvent represents a security issue/anomaly detected by AI when camera frames differ from trained dataset.
// This is NOT related to event-bus events - it's a security detection event.
type SecurityEvent struct {
	ID           string                 // UUID
	CameraID     string                 // Camera that detected the security issue
	EventType    string                 // Event type (e.g., "person_detected", "vehicle_detected", "anomaly_detected")
	Timestamp    time.Time              // When the security issue was detected
	Confidence   float64                // Detection confidence (0.0 to 1.0)
	BoundingBox  *aigateway.BoundingBox // Bounding box of detected object (nil if not applicable)
	Metadata     map[string]interface{} // Additional event metadata
	ClipPath     string                 // Path to associated video clip (if any)
	SnapshotPath string                 // Path to associated snapshot (if any)
}

// SecurityEventType constants - security issues detected by AI
const (
	SecurityEventTypePersonDetected   = "person_detected"
	SecurityEventTypeVehicleDetected  = "vehicle_detected"
	SecurityEventTypeObjectDetected   = "object_detected"
	SecurityEventTypeMotionDetected   = "motion_detected"
	SecurityEventTypeCustomDetected   = "custom_detected"
	SecurityEventTypeCameraObstructed = "camera_obstructed" // Critical: Camera view is blocked
	SecurityEventTypeAnomalyDetected  = "anomaly_detected"  // Adaptive AI anomaly - frame differs from trained dataset
)

// COCO class IDs for common objects
const (
	COCOClassPerson     = 0
	COCOClassBicycle    = 1
	COCOClassCar        = 2
	COCOClassMotorcycle = 3
	COCOClassAirplane   = 4
	COCOClassBus        = 5
	COCOClassTrain      = 6
	COCOClassTruck      = 7
)

// ClassIDToSecurityEventType maps COCO class IDs to security event types
var ClassIDToSecurityEventType = map[int]string{
	COCOClassPerson:     SecurityEventTypePersonDetected,
	COCOClassBicycle:    SecurityEventTypeVehicleDetected,
	COCOClassCar:        SecurityEventTypeVehicleDetected,
	COCOClassMotorcycle: SecurityEventTypeVehicleDetected,
	COCOClassBus:        SecurityEventTypeVehicleDetected,
	COCOClassTrain:      SecurityEventTypeVehicleDetected,
	COCOClassTruck:      SecurityEventTypeVehicleDetected,
}

// NewSecurityEvent creates a new security event with a generated UUID
func NewSecurityEvent() *SecurityEvent {
	return &SecurityEvent{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// ToEventState and SecurityEventFromEventState have been moved to impl/event_storage.go
// These functions are now internal to the state-mng implementation

// IsSignificant returns true if the security event is significant enough to be stored
func (e *SecurityEvent) IsSignificant(minConfidence float64) bool {
	return e.Confidence >= minConfidence
}

// GetSecurityEventTypeFromClassID returns the security event type for a given COCO class ID
func GetSecurityEventTypeFromClassID(classID int) string {
	if eventType, ok := ClassIDToSecurityEventType[classID]; ok {
		return eventType
	}
	return SecurityEventTypeObjectDetected
}
