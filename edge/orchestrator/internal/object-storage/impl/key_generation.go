package impl

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// GenerateDataUnitKey generates a unique key for a data unit.
// The key is organized by data type, device type, device ID, and date:
//   {dataType}/{deviceType}/{deviceID}/{YYYY-MM-DD}/{deviceID_timestamp_uuid.ext}
//
// Examples:
//   - Video clip: video_clips/camera/cam-001/2025-12-28/cam-001_120000_uuid.mp4
//   - Image: images/camera/cam-001/2025-12-28/cam-001_120000_uuid.jpg
//   - Sensor reading: sensor_readings/temperature/temp-001/2025-12-28/temp-001_120000_uuid.json
//   - Audio sample: audio_samples/microphone/mic-001/2025-12-28/mic-001_120000_uuid.wav
//
// Parameters:
//   - deviceID: The device identifier
//   - deviceType: The type of device (camera, sensor, audio_device, etc.)
//   - dataType: The type of data (video_clip, image, sensor_reading, etc.)
//   - isThumbnail: Whether this is a thumbnail version
//
// Returns a unique object storage key.
func GenerateDataUnitKey(deviceID types.DeviceID, deviceType types.DeviceType, dataType types.DataType, isThumbnail bool) string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	timestamp := now.Format("150405") // HHMMSS
	uuidStr := uuid.New().String()[:8] // Short UUID

	// Determine file extension based on data type
	var ext string
	switch dataType {
	case types.DataTypeVideoClip:
		ext = "mp4"
	case types.DataTypeVideoFrame:
		ext = "jpg"
	case types.DataTypeImage:
		ext = "jpg"
	case types.DataTypeSensorReading:
		ext = "json"
	case types.DataTypeAudioSample:
		ext = "wav"
	default:
		ext = "bin"
	}

	// Build filename
	filename := fmt.Sprintf("%s_%s_%s.%s", deviceID, timestamp, uuidStr, ext)
	if isThumbnail {
		// Insert _thumb before extension
		ext := filepath.Ext(filename)
		base := filename[:len(filename)-len(ext)]
		filename = fmt.Sprintf("%s_thumb%s", base, ext)
	}

	// Build full key path
	key := fmt.Sprintf("%s/%s/%s/%s/%s",
		dataType.String(),
		deviceType.String(),
		string(deviceID),
		dateStr,
		filename,
	)

	return key
}

// GenerateModelKey generates a key for a model artifact.
// The key is organized by device ID and model ID:
//   models/{deviceID}/{modelID}/{artifactType}.{ext}
//
// Examples:
//   - Model binary: models/cam-001/model-123/model.onnx
//   - Metadata: models/cam-001/model-123/metadata.json
//   - Manifest: models/cam-001/model-123/manifest.json
//
// Parameters:
//   - modelID: The model identifier
//   - deviceID: The device identifier
//   - artifactType: The type of artifact (model, metadata, manifest)
//
// Returns an object storage key for the model artifact.
func GenerateModelKey(modelID string, deviceID types.DeviceID, artifactType string) string {
	// Determine file extension based on artifact type
	var ext string
	switch artifactType {
	case "model":
		ext = "onnx" // Default model format, could be overridden
	case "metadata":
		ext = "json"
	case "manifest":
		ext = "json"
	default:
		ext = "bin"
	}

	key := fmt.Sprintf("models/%s/%s/%s.%s",
		string(deviceID),
		modelID,
		artifactType,
		ext,
	)

	return key
}

// GenerateSecurityEventAttachmentKey generates a key for a security event attachment.
// The key is organized by device ID and date:
//   security-events/{deviceID}/{YYYY-MM-DD}/{eventID}_{dataType}.{ext}
//
// Examples:
//   - Image attachment: security-events/cam-001/2025-12-28/event-123_image.jpg
//   - Sensor reading: security-events/temp-001/2025-12-28/event-123_sensor_reading.json
//
// Parameters:
//   - eventID: The security event identifier
//   - deviceID: The device identifier
//   - dataType: The type of data (image, sensor_reading, etc.)
//
// Returns an object storage key for the security event attachment.
func GenerateSecurityEventAttachmentKey(eventID string, deviceID types.DeviceID, dataType types.DataType) string {
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	// Determine file extension based on data type
	var ext string
	switch dataType {
	case types.DataTypeImage:
		ext = "jpg"
	case types.DataTypeSensorReading:
		ext = "json"
	case types.DataTypeAudioSample:
		ext = "wav"
	case types.DataTypeVideoClip:
		ext = "mp4"
	default:
		ext = "bin"
	}

	key := fmt.Sprintf("security-events/%s/%s/%s_%s.%s",
		string(deviceID),
		dateStr,
		eventID,
		dataType.String(),
		ext,
	)

	return key
}

