package camera

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_UpdateDatasetStatus(t *testing.T) {
	mgr, stateMgr := setupTestManager(t)
	defer stateMgr.Close()

	ctx := context.Background()

	// Register a camera first
	discovered := &DiscoveredCamera{
		ID:           "test-camera-1",
		Manufacturer: "Test",
		Model:        "Model",
		RTSPURLs:     []string{"rtsp://test/stream"},
		Capabilities: CameraCapabilities{HasVideoStreams: true},
		DiscoveredAt: time.Now(),
	}

	err := mgr.RegisterCamera(ctx, discovered)
	require.NoError(t, err)

	// Test updating dataset status for existing camera
	status := &CameraDatasetStatus{
		LabelCounts: map[string]int{
			"normal":   10,
			"threat":   2,
			"abnormal": 1,
		},
		LabeledSnapshotCount:  13,
		RequiredSnapshotCount: 50,
		SnapshotRequired:      true,
		LastSynced:            time.Now(),
	}

	mgr.UpdateDatasetStatus("test-camera-1", status)

	// Verify the status was updated
	camera, err := mgr.GetCamera("test-camera-1")
	require.NoError(t, err)
	require.NotNil(t, camera)
	assert.NotNil(t, camera.DatasetStatus)
	assert.Equal(t, 10, camera.DatasetStatus.LabelCounts["normal"])
	assert.Equal(t, 2, camera.DatasetStatus.LabelCounts["threat"])
	assert.Equal(t, 1, camera.DatasetStatus.LabelCounts["abnormal"])
	assert.Equal(t, 13, camera.DatasetStatus.LabeledSnapshotCount)
	assert.Equal(t, 50, camera.DatasetStatus.RequiredSnapshotCount)
	assert.True(t, camera.DatasetStatus.SnapshotRequired)

	// Test updating with different status
	newStatus := &CameraDatasetStatus{
		LabelCounts: map[string]int{
			"normal": 50,
		},
		LabeledSnapshotCount:  50,
		RequiredSnapshotCount: 50,
		SnapshotRequired:      false,
		LastSynced:            time.Now(),
	}

	mgr.UpdateDatasetStatus("test-camera-1", newStatus)

	// Verify the status was updated again
	camera, err = mgr.GetCamera("test-camera-1")
	require.NoError(t, err)
	assert.Equal(t, 50, camera.DatasetStatus.LabelCounts["normal"])
	assert.Equal(t, 50, camera.DatasetStatus.LabeledSnapshotCount)
	assert.False(t, camera.DatasetStatus.SnapshotRequired)

	// Test updating non-existent camera (should not panic, just do nothing)
	mgr.UpdateDatasetStatus("non-existent-camera", status)

	// Verify non-existent camera is not affected
	_, err = mgr.GetCamera("non-existent-camera")
	assert.Error(t, err)
}

func TestManager_UpdateDatasetStatus_MultipleCameras(t *testing.T) {
	mgr, stateMgr := setupTestManager(t)
	defer stateMgr.Close()

	ctx := context.Background()

	// Register multiple cameras
	cameras := []*DiscoveredCamera{
		{
			ID:           "camera-1",
			Manufacturer: "Test",
			Model:        "Model",
			RTSPURLs:     []string{"rtsp://test1/stream"},
			Capabilities: CameraCapabilities{HasVideoStreams: true},
			DiscoveredAt: time.Now(),
		},
		{
			ID:           "camera-2",
			Manufacturer: "Test",
			Model:        "Model",
			RTSPURLs:     []string{"rtsp://test2/stream"},
			Capabilities: CameraCapabilities{HasVideoStreams: true},
			DiscoveredAt: time.Now(),
		},
	}

	for _, discovered := range cameras {
		err := mgr.RegisterCamera(ctx, discovered)
		require.NoError(t, err)
	}

	// Update dataset status for camera-1
	status1 := &CameraDatasetStatus{
		LabelCounts: map[string]int{
			"normal": 10,
		},
		LabeledSnapshotCount:  10,
		RequiredSnapshotCount: 50,
		SnapshotRequired:      true,
		LastSynced:            time.Now(),
	}
	mgr.UpdateDatasetStatus("camera-1", status1)

	// Update dataset status for camera-2
	status2 := &CameraDatasetStatus{
		LabelCounts: map[string]int{
			"normal": 50,
		},
		LabeledSnapshotCount:  50,
		RequiredSnapshotCount: 50,
		SnapshotRequired:      false,
		LastSynced:            time.Now(),
	}
	mgr.UpdateDatasetStatus("camera-2", status2)

	// Verify both cameras have correct status
	cam1, err := mgr.GetCamera("camera-1")
	require.NoError(t, err)
	assert.Equal(t, 10, cam1.DatasetStatus.LabelCounts["normal"])
	assert.True(t, cam1.DatasetStatus.SnapshotRequired)

	cam2, err := mgr.GetCamera("camera-2")
	require.NoError(t, err)
	assert.Equal(t, 50, cam2.DatasetStatus.LabelCounts["normal"])
	assert.False(t, cam2.DatasetStatus.SnapshotRequired)
}
