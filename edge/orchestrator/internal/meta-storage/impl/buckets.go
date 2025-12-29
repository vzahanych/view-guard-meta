package impl

// Standard bucket names for meta-storage.
// These constants define the canonical bucket names used throughout the system.
// Bucket names follow a consistent naming convention:
// - Use snake_case
// - Be descriptive and clear
// - Avoid abbreviations unless widely understood

const (
	// ML Lifecycle and Model Management
	BucketMLLifecycle            = "ml_lifecycle"              // ML lifecycle state per device
	BucketPendingModelDeployments = "pending_model_deployments" // Pending model deployments
	BucketModelDeployments       = "model_deployments"          // Model deployment metadata (replaces deployed_models)

	// Device and Data Management
	BucketDevices     = "devices"      // Device metadata (replaces cameras)
	BucketDataUnits   = "data_units"   // Data unit metadata (replaces screenshots)
	BucketVideoClips  = "video_clips"  // Video clip metadata (replaces clips)
	BucketStorageState = "storage_state" // Storage entry metadata

	// Security and Events
	BucketSecurityEvents = "security_events" // Security event metadata
	BucketEventBus       = "event_bus"       // Event bus persistence
	BucketEventQueue     = "event_queue"     // Event queue
	BucketDeadLetterEvents = "dead_letter_events" // Dead letter events

	// Data Unit Requests
	BucketPendingDataUnitRequests = "pending_data_unit_requests" // Pending data unit capture requests (replaces pending_snapshot_requests)

	// Edge State and Capabilities
	BucketEdgeState       = "edge_state"        // Edge state metadata
	BucketEdgeStateHistory = "edge_state_history" // Edge state history
	BucketEdgeCapabilities = "edge_capabilities" // Edge capabilities metadata

	// Schema and Metadata
	BucketMeta = "_meta" // Schema version and metadata

	// Legacy bucket names (for migration)
	// These are kept for backward compatibility during migration
	BucketLegacyCameras                = "cameras"                 // Legacy: use devices
	BucketLegacyScreenshots             = "screenshots"              // Legacy: use data_units
	BucketLegacyClips                   = "clips"                    // Legacy: use video_clips
	BucketLegacyDeployedModels          = "deployed_models"           // Legacy: use model_deployments
	BucketLegacyPendingSnapshotRequests = "pending_snapshot_requests" // Legacy: use pending_data_unit_requests
)

// AllStandardBuckets returns a list of all standard bucket names that should exist.
// This is used for bucket initialization and validation.
func AllStandardBuckets() []string {
	return []string{
		BucketMLLifecycle,
		BucketPendingModelDeployments,
		BucketModelDeployments,
		BucketDevices,
		BucketDataUnits,
		BucketVideoClips,
		BucketStorageState,
		BucketSecurityEvents,
		BucketEventBus,
		BucketEventQueue,
		BucketDeadLetterEvents,
		BucketPendingDataUnitRequests,
		BucketEdgeState,
		BucketEdgeStateHistory,
		BucketEdgeCapabilities,
		BucketMeta,
	}
}

// AllLegacyBuckets returns a list of all legacy bucket names.
// These are used for migration purposes.
func AllLegacyBuckets() []string {
	return []string{
		BucketLegacyCameras,
		BucketLegacyScreenshots,
		BucketLegacyClips,
		BucketLegacyDeployedModels,
		BucketLegacyPendingSnapshotRequests,
	}
}

// BucketMigrationMap returns a map of old bucket names to new bucket names.
// This is used for bucket migration.
func BucketMigrationMap() map[string]string {
	return map[string]string{
		BucketLegacyCameras:                BucketDevices,
		BucketLegacyScreenshots:            BucketDataUnits,
		BucketLegacyClips:                  BucketVideoClips,
		BucketLegacyDeployedModels:         BucketModelDeployments,
		BucketLegacyPendingSnapshotRequests: BucketPendingDataUnitRequests,
	}
}

