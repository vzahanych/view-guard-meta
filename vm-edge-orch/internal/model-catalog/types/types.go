package types

import (
	"github.com/google/uuid"

	metastorage "github.com/vzahanych/view-guard-meta/vm-edge-orch/internal/meta-storage"
)

// RawModel represents a raw/baseline model with both metadata and storage information.
type RawModel struct {
	Meta            metastorage.RawModelMetadata `json:"meta"`
	ExistsInStorage bool                         `json:"exists_in_storage"`
}

// TrainedModel represents a trained model with both metadata and storage information.
type TrainedModel struct {
	Meta            metastorage.TrainedModelMetadata `json:"meta"`
	ExistsInStorage bool                             `json:"exists_in_storage"`
	SourceModel     *RawModel                        `json:"source_model,omitempty"` // Populated on request
}

// ModelType indicates whether a model is raw (baseline) or trained.
type ModelType string

const (
	ModelTypeRaw     ModelType = "raw"
	ModelTypeTrained ModelType = "trained"
)

// ModelInfo provides unified information about a model (either raw or trained).
type ModelInfo struct {
	ID           uuid.UUID     `json:"id"`
	Type         ModelType     `json:"type"` // "raw" or "trained"
	RawModel     *RawModel     `json:"raw_model,omitempty"`
	TrainedModel *TrainedModel `json:"trained_model,omitempty"`
}

