package database

// Schema contains SQLite table creation statements for User VM API

const (
	// Events table - cached event metadata from Edge
	CreateEventsTable = `
	CREATE TABLE IF NOT EXISTS events (
		event_id TEXT PRIMARY KEY,
		edge_id TEXT NOT NULL,
		camera_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		metadata TEXT, -- JSON metadata
		snapshot_path TEXT,
		clip_path TEXT,
		analyzed INTEGER DEFAULT 0, -- 0 = false, 1 = true
		severity REAL DEFAULT 0.0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (edge_id) REFERENCES edges(edge_id)
	);
	CREATE INDEX IF NOT EXISTS idx_events_edge_id ON events(edge_id);
	CREATE INDEX IF NOT EXISTS idx_events_camera_id ON events(camera_id);
	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
	CREATE INDEX IF NOT EXISTS idx_events_analyzed ON events(analyzed);
	`

	// Edges table - Edge Appliance registry
	CreateEdgesTable = `
	CREATE TABLE IF NOT EXISTS edges (
		edge_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		wireguard_public_key TEXT NOT NULL UNIQUE,
		wireguard_endpoint TEXT,
		last_seen INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'active', -- active, inactive, disconnected
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_edges_wireguard_public_key ON edges(wireguard_public_key);
	CREATE INDEX IF NOT EXISTS idx_edges_status ON edges(status);
	`

	// AI Models table - model catalog
	CreateAIModelsTable = `
	CREATE TABLE IF NOT EXISTS ai_models (
		model_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		type TEXT NOT NULL, -- cae, yolo, etc.
		base_model TEXT,
		training_dataset_id TEXT,
		model_file_path TEXT NOT NULL, -- Path to model.onnx
		metadata_file_path TEXT NOT NULL, -- Path to metadata.json
		status TEXT NOT NULL DEFAULT 'pending', -- pending, training, ready, deployed, failed
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (training_dataset_id) REFERENCES training_datasets(dataset_id)
	);
	CREATE INDEX IF NOT EXISTS idx_ai_models_type ON ai_models(type);
	CREATE INDEX IF NOT EXISTS idx_ai_models_status ON ai_models(status);
	CREATE INDEX IF NOT EXISTS idx_ai_models_training_dataset_id ON ai_models(training_dataset_id);
	`

	// Training Datasets table - dataset metadata
	CreateTrainingDatasetsTable = `
	CREATE TABLE IF NOT EXISTS training_datasets (
		dataset_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		edge_id TEXT NOT NULL,
		dataset_dir_path TEXT NOT NULL, -- Path to datasets/{dataset_id}/
		label_counts TEXT, -- JSON: {"normal": 100, "threat": 50, ...}
		total_images INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'pending', -- pending, ready, training, archived
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (edge_id) REFERENCES edges(edge_id)
	);
	CREATE INDEX IF NOT EXISTS idx_training_datasets_edge_id ON training_datasets(edge_id);
	CREATE INDEX IF NOT EXISTS idx_training_datasets_status ON training_datasets(status);
	`

	// CID Storage table - Filecoin/IPFS storage metadata (post-PoC)
	CreateCIDStorageTable = `
	CREATE TABLE IF NOT EXISTS cid_storage (
		cid_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		clip_path TEXT NOT NULL,
		cid TEXT NOT NULL UNIQUE, -- Content Identifier (IPFS/Filecoin)
		storage_provider TEXT NOT NULL, -- ipfs, filecoin, s3
		size_bytes INTEGER NOT NULL,
		uploaded_at INTEGER NOT NULL,
		retention_until INTEGER,
		FOREIGN KEY (event_id) REFERENCES events(event_id)
	);
	CREATE INDEX IF NOT EXISTS idx_cid_storage_event_id ON cid_storage(event_id);
	CREATE INDEX IF NOT EXISTS idx_cid_storage_cid ON cid_storage(cid);
	CREATE INDEX IF NOT EXISTS idx_cid_storage_retention_until ON cid_storage(retention_until);
	`

	// Telemetry Buffer table - aggregated telemetry from Edge
	CreateTelemetryBufferTable = `
	CREATE TABLE IF NOT EXISTS telemetry_buffer (
		telemetry_id TEXT PRIMARY KEY,
		edge_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		metrics_json TEXT NOT NULL, -- JSON telemetry data
		forwarded INTEGER DEFAULT 0, -- 0 = false, 1 = true
		created_at INTEGER NOT NULL,
		FOREIGN KEY (edge_id) REFERENCES edges(edge_id)
	);
	CREATE INDEX IF NOT EXISTS idx_telemetry_buffer_edge_id ON telemetry_buffer(edge_id);
	CREATE INDEX IF NOT EXISTS idx_telemetry_buffer_timestamp ON telemetry_buffer(timestamp);
	CREATE INDEX IF NOT EXISTS idx_telemetry_buffer_forwarded ON telemetry_buffer(forwarded);
	`
)

// AllTables returns all table creation statements in order
func AllTables() []string {
	return []string{
		CreateEdgesTable,
		CreateEventsTable,
		CreateTrainingDatasetsTable,
		CreateAIModelsTable,
		CreateCIDStorageTable,
		CreateTelemetryBufferTable,
	}
}

