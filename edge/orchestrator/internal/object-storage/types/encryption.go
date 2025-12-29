package types

import (
	"context"
)

// EncryptionProvider defines the interface for encryption providers.
// This allows different encryption implementations (KMS, hardware-backed, software).
type EncryptionProvider interface {
	// Encrypt encrypts the given data using the configured encryption method.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - data: The plaintext data to encrypt
	//
	// Returns:
	//   - Encrypted data (including IV/nonce if applicable)
	//   - An error if encryption fails
	Encrypt(ctx context.Context, data []byte) ([]byte, error)

	// Decrypt decrypts the given encrypted data using the configured decryption method.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - encryptedData: The encrypted data to decrypt
	//
	// Returns:
	//   - Plaintext data
	//   - An error if decryption fails
	Decrypt(ctx context.Context, encryptedData []byte) ([]byte, error)

	// GenerateKey generates a new encryption key (for per-object keys).
	// This is optional - some providers may use a single master key.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//
	// Returns:
	//   - Generated encryption key
	//   - An error if key generation fails
	GenerateKey(ctx context.Context) ([]byte, error)
}

// EncryptionMetadata contains metadata about encrypted objects.
// This is stored with the object to enable decryption.
type EncryptionMetadata struct {
	// Algorithm is the encryption algorithm used (e.g., "AES-256-GCM")
	Algorithm string

	// KeyID is the identifier of the encryption key used
	// For KMS providers, this is the KMS key ID
	// For software providers, this may be a key identifier or hash
	KeyID string

	// IV is the initialization vector (nonce) used for encryption
	// For GCM mode, this is the nonce
	// For CBC mode, this is the IV
	IV []byte

	// AdditionalData is optional additional authenticated data (AAD)
	// Used for authenticated encryption modes like GCM
	AdditionalData []byte
}

// IsEncrypted checks if an object is encrypted based on metadata.
// This checks if encryption metadata exists in the object metadata map.
func IsEncrypted(metadata map[string]string) bool {
	_, exists := metadata["encrypted"]
	return exists && metadata["encrypted"] == "true"
}

// GetEncryptionMetadata extracts encryption metadata from object metadata.
func GetEncryptionMetadata(metadata map[string]string) *EncryptionMetadata {
	if !IsEncrypted(metadata) {
		return nil
	}

	encMeta := &EncryptionMetadata{
		Algorithm: metadata["encryption_algorithm"],
		KeyID:     metadata["encryption_key_id"],
	}

	// IV is stored as base64-encoded string in metadata
	if ivStr, exists := metadata["encryption_iv"]; exists && ivStr != "" {
		// Note: Actual base64 decoding should be done by the encryption provider
		// This is just a placeholder - the provider will handle the encoding/decoding
		encMeta.IV = []byte(ivStr) // Provider should decode this
	}

	return encMeta
}

// SetEncryptionMetadata sets encryption metadata in object metadata map.
func SetEncryptionMetadata(metadata map[string]string, encMeta *EncryptionMetadata) {
	if encMeta == nil {
		return
	}

	metadata["encrypted"] = "true"
	metadata["encryption_algorithm"] = encMeta.Algorithm
	metadata["encryption_key_id"] = encMeta.KeyID

	// IV is stored as base64-encoded string
	if len(encMeta.IV) > 0 {
		// Note: Actual base64 encoding should be done by the encryption provider
		// This is just a placeholder - the provider will handle the encoding/decoding
		metadata["encryption_iv"] = string(encMeta.IV) // Provider should encode this
	}
}

