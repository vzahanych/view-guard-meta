package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM with the provided key
// Returns the encrypted data with the nonce prepended
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	// Validate key length (must be 32 bytes for AES-256)
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes for AES-256, got %d bytes", len(key))
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and prepend nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with the provided key
// Expects the nonce to be prepended to the ciphertext
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	// Validate key length
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes for AES-256, got %d bytes", len(key))
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// EncryptFile encrypts a file and writes the encrypted data to the output path
func EncryptFile(inputPath string, outputPath string, key []byte) error {
	// This is a placeholder - actual file encryption would read the file in chunks
	// For now, we'll provide the basic encryption functions
	return fmt.Errorf("file encryption not yet implemented - use Encrypt with file I/O")
}

// DecryptFile decrypts an encrypted file and writes the plaintext to the output path
func DecryptFile(inputPath string, outputPath string, key []byte) error {
	// This is a placeholder - actual file decryption would read the file in chunks
	// For now, we'll provide the basic decryption functions
	return fmt.Errorf("file decryption not yet implemented - use Decrypt with file I/O")
}

