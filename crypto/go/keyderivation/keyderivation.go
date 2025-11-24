package keyderivation

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// Argon2Params holds parameters for Argon2id key derivation
type Argon2Params struct {
	Memory      uint32 // Memory cost in KiB
	Iterations  uint32 // Number of iterations
	Parallelism uint8  // Number of threads
	KeyLength   uint32 // Length of derived key in bytes
}

// DefaultArgon2Params returns recommended Argon2id parameters
// These are suitable for most use cases and provide good security
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Memory:      64 * 1024, // 64 MiB
		Iterations:  3,         // 3 iterations
		Parallelism: 4,         // 4 threads
		KeyLength:   32,        // 32 bytes (256 bits) for AES-256
	}
}

// DeriveKey derives an encryption key from a user secret using Argon2id
// Returns a 32-byte key suitable for AES-256 encryption
func DeriveKey(secret []byte, salt []byte) ([]byte, error) {
	params := DefaultArgon2Params()
	return DeriveKeyWithParams(secret, salt, params)
}

// DeriveKeyWithParams derives an encryption key with custom Argon2id parameters
func DeriveKeyWithParams(secret []byte, salt []byte, params Argon2Params) ([]byte, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("secret cannot be empty")
	}

	if len(salt) < 16 {
		return nil, fmt.Errorf("salt must be at least 16 bytes, got %d bytes", len(salt))
	}

	// Derive key using Argon2id
	key := argon2.IDKey(secret, salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)

	return key, nil
}

// GenerateSalt generates a random salt for key derivation
// Returns a 32-byte salt (recommended size)
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// DeriveKeyFromString derives a key from a string secret (e.g., passphrase)
// The secret is converted to bytes and a salt is generated
func DeriveKeyFromString(secret string, salt []byte) ([]byte, error) {
	return DeriveKey([]byte(secret), salt)
}

// HashSecret creates a hash of the secret for storage/identification purposes
// This hash can be used to identify which key was used without storing the secret
func HashSecret(secret []byte) string {
	hash := sha256.Sum256(secret)
	return hex.EncodeToString(hash[:])
}

