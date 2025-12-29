package impl

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

func TestGenerateDataUnitKey(t *testing.T) {
	deviceID := types.DeviceID("camera-001")
	deviceType := types.DeviceTypeCamera
	dataType := types.DataTypeImage

	key := GenerateDataUnitKey(deviceID, deviceType, dataType, false)

	// Verify key structure: {dataType}/{deviceType}/{deviceID}/{YYYY-MM-DD}/{filename}
	parts := strings.Split(key, "/")
	assert.Equal(t, 5, len(parts))
	assert.Equal(t, "image", parts[0])
	assert.Equal(t, "camera", parts[1])
	assert.Equal(t, "camera-001", parts[2])
	
	// Verify date format (YYYY-MM-DD)
	_, err := time.Parse("2006-01-02", parts[3])
	assert.NoError(t, err)

	// Verify filename contains device ID and extension
	filename := parts[4]
	assert.Contains(t, filename, "camera-001")
	assert.True(t, strings.HasSuffix(filename, ".jpg"))
}

func TestGenerateDataUnitKey_Thumbnail(t *testing.T) {
	deviceID := types.DeviceID("camera-001")
	deviceType := types.DeviceTypeCamera
	dataType := types.DataTypeImage

	key := GenerateDataUnitKey(deviceID, deviceType, dataType, true)

	// Verify thumbnail flag in filename
	parts := strings.Split(key, "/")
	filename := parts[4]
	assert.Contains(t, filename, "_thumb")
	assert.True(t, strings.HasSuffix(filename, ".jpg"))
}

func TestGenerateDataUnitKey_AllDataTypes(t *testing.T) {
	deviceID := types.DeviceID("device-001")
	deviceType := types.DeviceTypeCamera

	testCases := []struct {
		dataType types.DataType
		expectedExt string
	}{
		{types.DataTypeVideoClip, "mp4"},
		{types.DataTypeVideoFrame, "jpg"},
		{types.DataTypeImage, "jpg"},
		{types.DataTypeSensorReading, "json"},
		{types.DataTypeAudioSample, "wav"},
	}

	for _, tc := range testCases {
		key := GenerateDataUnitKey(deviceID, deviceType, tc.dataType, false)
		assert.True(t, strings.HasSuffix(key, "."+tc.expectedExt), "Expected extension %s for data type %s", tc.expectedExt, tc.dataType)
		assert.Contains(t, key, tc.dataType.String())
	}
}

func TestGenerateModelKey(t *testing.T) {
	modelID := "yolov8-detection-v1"
	deviceID := types.DeviceID("camera-001")

	// Test model artifact
	key := GenerateModelKey(modelID, deviceID, "model")
	assert.Equal(t, "models/camera-001/yolov8-detection-v1/model.onnx", key)

	// Test metadata artifact
	key = GenerateModelKey(modelID, deviceID, "metadata")
	assert.Equal(t, "models/camera-001/yolov8-detection-v1/metadata.json", key)

	// Test manifest artifact
	key = GenerateModelKey(modelID, deviceID, "manifest")
	assert.Equal(t, "models/camera-001/yolov8-detection-v1/manifest.json", key)
}

func TestGenerateSecurityEventAttachmentKey(t *testing.T) {
	eventID := "event-123"
	deviceID := types.DeviceID("camera-001")
	dataType := types.DataTypeImage

	key := GenerateSecurityEventAttachmentKey(eventID, deviceID, dataType)

	// Verify key structure: security-events/{deviceID}/{YYYY-MM-DD}/{eventID}_{dataType}.{ext}
	parts := strings.Split(key, "/")
	assert.Equal(t, 4, len(parts))
	assert.Equal(t, "security-events", parts[0])
	assert.Equal(t, "camera-001", parts[1])
	
	// Verify date format
	_, err := time.Parse("2006-01-02", parts[2])
	assert.NoError(t, err)

	// Verify filename
	filename := parts[3]
	assert.Contains(t, filename, "event-123")
	assert.Contains(t, filename, "image")
	assert.True(t, strings.HasSuffix(filename, ".jpg"))
}

func TestGenerateSecurityEventAttachmentKey_AllDataTypes(t *testing.T) {
	eventID := "event-123"
	deviceID := types.DeviceID("device-001")

	testCases := []struct {
		dataType types.DataType
		expectedExt string
	}{
		{types.DataTypeImage, "jpg"},
		{types.DataTypeSensorReading, "json"},
		{types.DataTypeAudioSample, "wav"},
		{types.DataTypeVideoClip, "mp4"},
	}

	for _, tc := range testCases {
		key := GenerateSecurityEventAttachmentKey(eventID, deviceID, tc.dataType)
		assert.True(t, strings.HasSuffix(key, "."+tc.expectedExt), "Expected extension %s for data type %s", tc.expectedExt, tc.dataType)
		assert.Contains(t, key, "security-events")
		assert.Contains(t, key, eventID)
	}
}

