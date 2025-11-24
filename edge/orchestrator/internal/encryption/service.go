package encryption

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/vzahanych/view-guard-meta/crypto/go/encryption"
	"github.com/vzahanych/view-guard-meta/crypto/go/keyderivation"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/service"
)

// Service provides encryption functionality for clips
type Service struct {
	*service.ServiceBase
	config      *config.EncryptionConfig
	logger      *logger.Logger
	userSecret  []byte
	salt        []byte
	derivedKey  []byte
	keyHash     string
	mu          sync.RWMutex
}

// EncryptionMetadata contains metadata about an encrypted clip
type EncryptionMetadata struct {
	KeyHash     string // Hash of the user secret (for identification)
	Salt        string // Salt used for key derivation (hex encoded)
	Algorithm   string // Encryption algorithm (e.g., "AES-256-GCM")
	KeyDerivation string // Key derivation function (e.g., "Argon2id")
}

// NewService creates a new encryption service
func NewService(cfg *config.EncryptionConfig, log *logger.Logger) *Service {
	return &Service{
		ServiceBase: service.NewServiceBase("encryption-service", log),
		config:      cfg,
		logger:      log,
	}
}

// Start starts the encryption service
func (s *Service) Start(ctx context.Context) error {
	s.GetStatus().SetStatus(service.StatusRunning)

	if !s.config.Enabled {
		s.LogInfo("Encryption service is disabled")
		return nil
	}

	// Load or generate user secret
	if err := s.loadOrGenerateSecret(); err != nil {
		return fmt.Errorf("failed to load secret: %w", err)
	}

	// Derive encryption key
	if err := s.deriveKey(); err != nil {
		return fmt.Errorf("failed to derive key: %w", err)
	}

	s.LogInfo("Encryption service started",
		"key_hash", s.keyHash,
	)

	return nil
}

// Stop stops the encryption service
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear sensitive data from memory
	s.userSecret = nil
	s.derivedKey = nil

	s.LogInfo("Encryption service stopped")
	s.GetStatus().SetStatus(service.StatusStopped)
	return nil
}

// loadOrGenerateSecret loads the user secret from config or generates a new one
func (s *Service) loadOrGenerateSecret() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// For PoC, we'll use a configurable secret or generate one
	// In production, this would be securely stored and loaded
	if s.config.UserSecret != "" {
		s.userSecret = []byte(s.config.UserSecret)
	} else {
		// Generate a random secret for PoC (not recommended for production)
		secret := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, secret); err != nil {
			return fmt.Errorf("failed to generate secret: %w", err)
		}
		s.userSecret = secret
		s.LogError("Generated random secret for PoC - not suitable for production", fmt.Errorf("warning"), "note", "this is a PoC-only feature")
	}

	// Generate or load salt
	if s.config.Salt != "" {
		saltBytes, err := hex.DecodeString(s.config.Salt)
		if err != nil {
			return fmt.Errorf("failed to decode salt: %w", err)
		}
		s.salt = saltBytes
	} else if s.config.SaltPath != "" {
		// Try to load salt from file (without lock since we already hold it)
		if err := s.loadSaltFromFileUnlocked(s.config.SaltPath); err != nil {
			// If file doesn't exist, generate new salt and save it
			salt, err := keyderivation.GenerateSalt()
			if err != nil {
				return fmt.Errorf("failed to generate salt: %w", err)
			}
			s.salt = salt
			// Save the generated salt (without lock since we already hold it)
			if err := s.saveSaltToFileUnlocked(s.config.SaltPath); err != nil {
				s.LogError("Failed to save salt to file", err)
			}
		}
	} else {
		// Generate new salt
		salt, err := keyderivation.GenerateSalt()
		if err != nil {
			return fmt.Errorf("failed to generate salt: %w", err)
		}
		s.salt = salt
	}

	// Compute key hash for identification
	s.keyHash = keyderivation.HashSecret(s.userSecret)

	return nil
}

// deriveKey derives the encryption key from the user secret
func (s *Service) deriveKey() error {
	s.mu.RLock()
	secret := s.userSecret
	salt := s.salt
	s.mu.RUnlock()

	key, err := keyderivation.DeriveKey(secret, salt)
	if err != nil {
		return fmt.Errorf("failed to derive key: %w", err)
	}

	s.mu.Lock()
	s.derivedKey = key
	s.mu.Unlock()

	return nil
}

// EncryptClip encrypts a clip file and returns the path to the encrypted file
func (s *Service) EncryptClip(ctx context.Context, clipPath string) (string, error) {
	if !s.config.Enabled {
		return clipPath, nil // Return original path if encryption is disabled
	}

	s.mu.RLock()
	key := s.derivedKey
	s.mu.RUnlock()

	if key == nil {
		return "", fmt.Errorf("encryption key not available")
	}

	// Read the clip file
	plaintext, err := os.ReadFile(clipPath)
	if err != nil {
		return "", fmt.Errorf("failed to read clip file: %w", err)
	}

	// Encrypt the data
	ciphertext, err := encryption.Encrypt(plaintext, key)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt clip: %w", err)
	}

	// Write encrypted data to a new file
	encryptedPath := clipPath + ".encrypted"
	if err := os.WriteFile(encryptedPath, ciphertext, 0644); err != nil {
		return "", fmt.Errorf("failed to write encrypted file: %w", err)
	}

	s.LogDebug("Clip encrypted",
		"original_path", clipPath,
		"encrypted_path", encryptedPath,
		"original_size", len(plaintext),
		"encrypted_size", len(ciphertext),
	)

	return encryptedPath, nil
}

// DecryptClip decrypts an encrypted clip file
func (s *Service) DecryptClip(ctx context.Context, encryptedPath string) ([]byte, error) {
	if !s.config.Enabled {
		// If encryption is disabled, just read the file
		return os.ReadFile(encryptedPath)
	}

	s.mu.RLock()
	key := s.derivedKey
	s.mu.RUnlock()

	if key == nil {
		return nil, fmt.Errorf("encryption key not available")
	}

	// Read the encrypted file
	ciphertext, err := os.ReadFile(encryptedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read encrypted file: %w", err)
	}

	// Decrypt the data
	plaintext, err := encryption.Decrypt(ciphertext, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt clip: %w", err)
	}

	return plaintext, nil
}

// GetEncryptionMetadata returns metadata about the encryption configuration
func (s *Service) GetEncryptionMetadata() *EncryptionMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &EncryptionMetadata{
		KeyHash:        s.keyHash,
		Salt:            hex.EncodeToString(s.salt),
		Algorithm:       "AES-256-GCM",
		KeyDerivation:   "Argon2id",
	}
}

// GetKeyHash returns the hash of the user secret (for identification only)
func (s *Service) GetKeyHash() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keyHash
}

// EncryptData encrypts arbitrary data in memory
func (s *Service) EncryptData(data []byte) ([]byte, error) {
	if !s.config.Enabled {
		return data, nil // Return original data if encryption is disabled
	}

	s.mu.RLock()
	key := s.derivedKey
	s.mu.RUnlock()

	if key == nil {
		return nil, fmt.Errorf("encryption key not available")
	}

	return encryption.Encrypt(data, key)
}

// DecryptData decrypts arbitrary data in memory
func (s *Service) DecryptData(data []byte) ([]byte, error) {
	if !s.config.Enabled {
		return data, nil // Return original data if encryption is disabled
	}

	s.mu.RLock()
	key := s.derivedKey
	s.mu.RUnlock()

	if key == nil {
		return nil, fmt.Errorf("encryption key not available")
	}

	return encryption.Decrypt(data, key)
}

// SaveSaltToFile saves the salt to a file for persistence
func (s *Service) SaveSaltToFile(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveSaltToFileUnlocked(path)
}

// LoadSaltFromFile loads the salt from a file
// This method acquires a lock - use loadSaltFromFileUnlocked if you already hold the lock
func (s *Service) LoadSaltFromFile(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadSaltFromFileUnlocked(path)
}

// loadSaltFromFileUnlocked loads the salt from a file without acquiring a lock
// Caller must hold s.mu.Lock()
func (s *Service) loadSaltFromFileUnlocked(path string) error {
	saltHex, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read salt file: %w", err)
	}

	salt, err := hex.DecodeString(string(saltHex))
	if err != nil {
		return fmt.Errorf("failed to decode salt: %w", err)
	}

	s.salt = salt
	return nil
}

// saveSaltToFileUnlocked saves the salt to a file without acquiring a lock
// Caller must hold s.mu.Lock()
func (s *Service) saveSaltToFileUnlocked(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write salt as hex string
	saltHex := hex.EncodeToString(s.salt)
	if err := os.WriteFile(path, []byte(saltHex), 0600); err != nil {
		return fmt.Errorf("failed to write salt file: %w", err)
	}

	return nil
}

