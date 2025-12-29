package types

import "time"

// ModelManifest represents the manifest for a model deployment.
// This contains metadata about the model, its artifacts, and deployment information.
// Note: This is a simplified version. A full ModelManifest may be defined in state-mng types.
type ModelManifest struct {
	// ModelID is the unique identifier for the model
	ModelID string

	// DeviceID is the device this model is deployed to
	DeviceID DeviceID

	// Version is the version of the model (e.g., "v1.0.0")
	Version string

	// TargetRuntime is the target runtime (e.g., "OpenVINO", "ONNX")
	TargetRuntime string

	// ProtocolVersion is the protocol version
	ProtocolVersion string

	// SchemaVersion is the schema version
	SchemaVersion string

	// ArtifactHashes is a map of artifact type to SHA-256 hash
	// This allows verification of artifact integrity
	ArtifactHashes map[string]string

	// CreatedAt is when the manifest was created
	CreatedAt time.Time

	// Additional metadata (compatibility constraints, training provenance, etc.)
	Metadata map[string]interface{}
}

// ModelArtifacts represents the complete set of artifacts for a model.
// This is returned by LoadModelArtifacts.
type ModelArtifacts struct {
	// ModelID is the unique identifier for the model
	ModelID string

	// DeviceID is the device this model is deployed to
	DeviceID DeviceID

	// Version is the version of the model (e.g., "v1.0.0")
	Version string

	// Model is the model binary (the actual ML model file)
	Model []byte

	// Metadata is the model metadata (JSON)
	Metadata []byte

	// Manifest is the model manifest (JSON)
	Manifest []byte

	// Hashes is a map of artifact type to SHA-256 hash
	// This is used for integrity verification
	Hashes map[string]string

	// CreatedAt is when the artifacts were created
	CreatedAt time.Time
}

// ModelVersion represents a model version for a specific device.
// This is used for version tracking and management.
type ModelVersion struct {
	// ModelID is the unique identifier for the model
	ModelID string

	// DeviceID is the device this model is deployed to
	DeviceID DeviceID

	// Version is the version string (e.g., "v1.0.0")
	Version string

	// CreatedAt is when this model version was created
	CreatedAt time.Time

	// ArtifactCount is the number of artifacts stored for this model version
	ArtifactCount int

	// TotalSizeBytes is the total size of all artifacts in bytes
	TotalSizeBytes int64
}
