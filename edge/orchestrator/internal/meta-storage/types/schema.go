package types

import (
	"context"
	"time"
)

// SchemaVersion represents a schema version in the storage system.
// Schema versions are used for migration tracking and compatibility checks.
// This is stored in the _meta bucket and tracks which migrations have been applied.
type SchemaVersion struct {
	// Version is the schema version number (incrementing integer).
	// Version 0 indicates no migrations have been applied (initial state).
	Version int

	// AppliedAt is when this schema version was applied.
	AppliedAt time.Time

	// Description is a human-readable description of what this schema version includes.
	Description string
}

// SchemaMigration represents a schema migration that can be applied or rolled back.
// Migrations are used to evolve the storage schema over time while maintaining compatibility.
//
// Schema migrations allow the database schema to evolve over time without losing data.
// Each migration should:
//   - Be idempotent (safe to run multiple times) for the Up method
//   - Handle both new and existing data
//   - Not break existing functionality
//   - Provide a Down method for rollback (though rollback is optional)
//
// Example migration:
//   type MigrationV1ToV2 struct {
//       provider MetaStorageProvider
//       logger   *zap.Logger
//   }
//
//   func (m *MigrationV1ToV2) Version() int { return 2 }
//   func (m *MigrationV1ToV2) Description() string { return "Add version field to ML lifecycle state" }
//   func (m *MigrationV1ToV2) Up(ctx context.Context) error {
//       // Migration logic here
//       return nil
//   }
//   func (m *MigrationV1ToV2) Down(ctx context.Context) error {
//       // Rollback logic here (optional)
//       return nil
//   }
type SchemaMigration interface {
	// Up applies the migration (upgrade to new schema version).
	// This method should be idempotent (safe to run multiple times).
	//
	// The migration should:
	//   - Check if the migration has already been applied (if possible)
	//   - Apply the migration changes
	//   - Handle errors gracefully
	//   - Not modify data that has already been migrated
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns an error if:
	//   - The migration cannot be applied
	//   - The provider operation fails
	//   - Data corruption is detected
	Up(ctx context.Context) error

	// Down rolls back the migration (downgrade to previous schema version).
	// This method is optional but recommended for production systems.
	//
	// The rollback should:
	//   - Reverse the changes made by Up()
	//   - Handle errors gracefully
	//   - Not modify data that has already been rolled back
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns an error if:
	//   - The rollback cannot be applied
	//   - The provider operation fails
	//   - Data corruption is detected
	//
	// Note: Rollback should be used with caution and only when necessary.
	Down(ctx context.Context) error

	// Version returns the schema version number this migration applies.
	// Migrations are applied in ascending order of version number.
	// Version numbers should be sequential (1, 2, 3, ...) with no gaps.
	Version() int

	// Description returns a human-readable description of what this migration does.
	// This is used for logging and debugging purposes.
	Description() string
}

