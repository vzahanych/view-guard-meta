/*
Package objectstorage provides a unified, provider-agnostic interface for object storage operations.

The Object Storage service manages binary objects (images, video clips, sensor data, model artifacts, etc.)
stored in object storage backends. It abstracts away the details of storage provider implementations,
allowing the system to work with any storage backend (MinIO, S3, local filesystem, etc.).

# Architecture Overview

The storage service follows a provider-agnostic architecture pattern similar to vm-gateway:

	┌─────────────────────────────────────────────────────────┐
	│          ObjectStorageService Interface                   │
	│  (Unified API for Object Storage Operations)             │
	└─────────────────────────────────────────────────────────┘
	                          │
	          ┌───────────────┴───────────────┐
	          │                                 │
	┌─────────▼──────────┐         ┌───────────▼──────────┐
	│  MinIO Provider    │         │  S3 Provider        │
	│  (Current)         │         │  (Future)            │
	│                    │         │                      │
	│  - S3-compatible    │         │  - AWS S3           │
	│  - Local/Remote    │         │  - Cloud storage     │
	└────────────────────┘         └──────────────────────┘

The service is composed of:
  - ObjectStorageService interface: High-level operations (store, load, delete, lifecycle)
  - ObjectStorageProvider interface: Low-level storage operations (objects, metadata)
  - Provider implementations: MinIO (current), S3, filesystem (future)
  - Managers: Quota, Retention, Integrity, Health Monitoring

# Provider-Agnostic Design

The storage service is designed to be provider-agnostic:

  - Storage providers: MinIO (current), S3, local filesystem (future)
  - Device types: Cameras, sensors, audio devices, and other IoT devices
  - Data types: Video clips, images, sensor readings, audio samples, model artifacts, etc.

This allows the system to:
  - Switch storage providers without changing application code
  - Support different deployment scenarios (local MinIO, cloud S3, embedded filesystem)
  - Add new providers by implementing the ObjectStorageProvider interface

The ObjectStorageProvider interface defines low-level operations:
  - StoreObject, LoadObject, DeleteObject
  - ListObjects, GetObjectMetadata
  - HealthCheck

The ObjectStorageService interface defines high-level operations:
  - Data unit storage (device-agnostic)
  - Model artifact storage
  - Security event attachment storage
  - Quota management
  - Retention policies
  - Integrity verification
  - Health monitoring

# Device-Agnostic Design

The storage service is device-agnostic, supporting various device types:

  - Cameras (IP cameras, USB cameras)
  - Sensors (temperature, motion, door sensors, etc.)
  - Audio devices (microphones, speakers)
  - Other IoT devices

All device-specific operations use generic DeviceID and DeviceType:
  - DeviceID: Unique identifier for any device
  - DeviceType: Type of device (camera, sensor, audio, etc.)

# Configuration

The storage service uses a provider-agnostic configuration structure.

## Basic Configuration (MinIO)

	provider: minio
	endpoint: localhost:9000
	access_key: minioadmin
	secret_key: minioadmin
	region: us-east-1

## Advanced Configuration (MinIO with Quota, Retention, and Encryption)

	provider: minio
	endpoint: localhost:9000
	access_key: minioadmin
	secret_key: minioadmin
	region: us-east-1
	quota:
	  max_size_mb: 100000
	  warning_threshold_percent: 80
	  full_threshold_percent: 95
	retention:
	  dataset_retention_days: 30
	  event_retention_days: 7
	  model_retention_versions: 2
	  model_retention_grace_period_days: 7
	  cleanup_interval_hours: 6
	encryption:
	  enabled: true
	  provider: software
	  algorithm: AES-256-GCM
	  key_source: /etc/view-guard/encryption.key
	minio:
	  bucket: edge-storage
	  use_ssl: false
	  insecure_skip_verify: false

## Future S3 Configuration

	provider: s3
	endpoint: s3.amazonaws.com
	access_key: AKIAIOSFODNN7EXAMPLE
	secret_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
	region: us-east-1
	s3:
	  bucket: view-guard-edge-storage

# Usage Examples

## Basic Usage with Dependency Injection (Fx)

	import (
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
		"go.uber.org/fx"
	)

	// In your Fx module
	var Module = fx.Module("object-storage",
		fx.Provide(objectstorage.ObjectStorageProvider),
	)

	// The service will be automatically started and stopped by Fx

## Manual Creation

	import (
		"context"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
		"go.uber.org/zap"
	)

	cfg := &types.ObjectStorageConfig{
		Provider: "minio",
		Endpoint: "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Region: "us-east-1",
		QuotaConfig: &types.QuotaConfig{
			MaxSizeMB: 100000,
			WarningThresholdPercent: 80,
			FullThresholdPercent: 95,
		},
		RetentionConfig: &types.RetentionConfig{
			DatasetRetentionDays: 30,
			EventRetentionDays: 7,
			CleanupIntervalHours: 6,
		},
	}

	logger := zap.NewNop()
	store, err := objectstorage.NewObjectStorageService(context.Background(), cfg, logger)
	if err != nil {
		// handle error
	}

	// Start the service
	if err := store.Start(context.Background()); err != nil {
		// handle error
	}
	defer store.Stop(context.Background())

## Storing a Data Unit

	import (
		"bytes"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	)

	deviceID := types.DeviceID("camera-001")
	deviceType := types.DeviceTypeCamera
	dataType := types.DataTypeImage
	
	// Generate key
	key := store.GenerateDataUnitKey(deviceID, deviceType, dataType, false)
	
	// Store object
	imageData := []byte("...") // image bytes
	r := bytes.NewReader(imageData)
	if err := store.StoreDataUnit(ctx, deviceID, deviceType, dataType, key, r, int64(len(imageData)), "image/jpeg"); err != nil {
		if errors.Is(err, objectstorage.ErrQuotaExceeded) {
			// Handle quota exceeded
		}
		// handle error
	}

## Loading a Data Unit

	rc, err := store.LoadDataUnit(ctx, key)
	if err != nil {
		if errors.Is(err, objectstorage.ErrObjectNotFound) {
			// Handle object not found
		}
		if errors.Is(err, objectstorage.ErrCorruptionDetected) {
			// Handle corruption detected
		}
		// handle error
	}
	defer rc.Close()
	
	// Read data from rc (automatically decrypted if encrypted)

## Storing Model Artifacts

	import (
		"encoding/json"
		"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
	)

	modelID := "yolov8-detection-v1"
	deviceID := types.DeviceID("camera-001")
	
	// Create manifest
	manifest := &types.ModelManifest{
		ModelID:        modelID,
		DeviceID:       deviceID,
		Version:        "v1.0.0",
		TargetRuntime:  "OpenVINO",
		ProtocolVersion: "1.0",
		SchemaVersion:   "1.0",
		ArtifactHashes: make(map[string]string),
		CreatedAt:      time.Now(),
		Metadata:       make(map[string]interface{}),
	}
	
	// Prepare artifacts
	artifacts := map[string][]byte{
		"model":    modelBytes,    // Model binary
		"metadata": metadataBytes, // Model metadata JSON
		"manifest": manifestBytes,  // Manifest JSON
	}
	
	// Store artifacts
	if err := store.StoreModelArtifacts(ctx, modelID, deviceID, manifest, artifacts); err != nil {
		// handle error
	}

## Loading Model Artifacts

	artifacts, err := store.LoadModelArtifacts(ctx, modelID, deviceID)
	if err != nil {
		// handle error
	}
	
	// Use artifacts
	model := artifacts.Model
	metadata := artifacts.Metadata
	manifest := artifacts.Manifest
	hashes := artifacts.Hashes // Integrity hashes

## Storing Security Event Attachments

	eventID := "event-123"
	deviceID := types.DeviceID("camera-001")
	dataType := types.DataTypeImage
	
	// Store attachment (automatically optimized if quota is high)
	key, err := store.StoreSecurityEventAttachment(ctx, eventID, deviceID, dataType, imageData)
	if err != nil {
		// handle error
	}
	
	// Store key in meta-storage for event reference

## Health Monitoring

	health := store.HealthSnapshot()
	if health.Status == types.HealthStatusWarning {
		// Storage is approaching quota limit
		logger.Warn("Storage quota warning",
			zap.Int64("used_mb", health.Quota.Used/1024/1024),
			zap.Int64("limit_mb", health.Quota.Limit/1024/1024),
		)
	}

# Production Features

## Quota Management

The service enforces storage quotas to prevent disk space exhaustion:

  - Warning threshold (default: 80%): Emits warnings but allows operations
  - Full threshold (default: 95%): Rejects new storage operations
  - Gradual backpressure: Throttles large objects at 90-95% usage
  - Automatic optimization: Compresses attachments at 90%+, reduces quality at 95%+
  - Quota tracking: Real-time usage tracking via ListObjects
  - Object counting: Tracks object counts by data type

## Retention Policies

The service automatically cleans up expired objects:

  - Dataset retention: Objects retained for N days after upload completion (default: 30 days)
  - Event retention: Attachments retained for N days after VM acknowledgment (default: 7 days)
  - Model version retention: Keeps last N versions per device (default: 2 versions)
  - Grace period: Model artifacts have grace period after purge eligibility (default: 7 days)
  - Automatic cleanup: Runs periodically (default: every 6 hours)
  - Orphaned attachment cleanup: Removes attachments not referenced by events

## Integrity Verification

The service verifies object integrity:

  - Hash calculation: SHA-256 hash calculated on store (before encryption)
  - Hash verification: Hash verified on load (after decryption)
  - Corruption detection: Periodic integrity checks (daily by default)
  - Integrity metadata: Hash stored in object metadata for verification
  - Corruption events: Emits events when corruption is detected

## Encryption at Rest

The service supports encryption for sensitive data:

  - Encryption providers: KMS, hardware-backed, software (default: software)
  - Algorithms: AES-256-GCM (default), AES-128-GCM, ChaCha20-Poly1305
  - Automatic encryption: Encrypts sensitive data types when enabled
  - Encryption metadata: Stores encryption metadata in object metadata
  - Transparent decryption: Automatically decrypts on load
  - Data types encrypted: Video clips, images, sensor readings, audio samples, security event attachments, model artifacts

## Health Monitoring

The service provides comprehensive health monitoring:

  - Health snapshot: Real-time status of storage health
  - Quota status: Current usage and thresholds
  - Integrity status: Corruption detection results
  - Provider health: Provider-specific health status
  - Object counts: Counts by data type
  - Cleanup statistics: Last cleanup time and statistics
  - Operational metrics: Operation counts, latencies, error rates

# Error Handling

The service uses sentinel errors for programmatic error handling:

  - ErrNotInitialized: Service not properly initialized
  - ErrAlreadyStarted: Service already started
  - ErrQuotaExceeded: Storage quota exceeded
  - ErrObjectNotFound: Object not found
  - ErrCorruptionDetected: Storage corruption detected

Use errors.Is() to check for specific error conditions:

	if errors.Is(err, objectstorage.ErrQuotaExceeded) {
		// Handle quota exceeded
	}

# Lifecycle Management

The service follows the vm-gateway lifecycle pattern:

  - Start(): Initializes provider, verifies connectivity, starts background tasks
  - Stop(): Stops background tasks, closes connections gracefully
  - Service owns lifecycle: The service manages the lifecycle of sub-components (providers, managers)

# Observability

The service emits operational events:

  - storage.warning: Quota usage 80-90%
  - storage.full: Quota usage >95%
  - storage.quota_exceeded: Quota exceeded during write operation
  - storage.cleanup_started, storage.cleanup_completed: Retention cleanup events
  - storage.corruption_detected: Integrity failures

The service tracks operational metrics:

  - Operation counts: Store, load, delete operations by data type
  - Latency percentiles: P50, P95, P99 latencies per operation type
  - Error rates: Error counts and rates per operation type
  - Quota history: Quota utilization samples over time
  - Cleanup statistics: Objects deleted, space freed, data types processed

# Recent Refactoring

This package has been refactored to be fully provider-agnostic and device-agnostic:

  - Provider-agnostic: Clean interface/implementation separation
  - Device-agnostic: Unified DeviceID/DeviceType abstraction
  - Production features: Quota, retention, integrity, health monitoring
  - Lifecycle management: Proper Start/Stop pattern
  - Observability: Health snapshot API and event emission
*/
package objectstorage

