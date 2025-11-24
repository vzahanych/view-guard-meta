package encryption

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/config"
	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/logger"
)

func setupTestService(t *testing.T) *Service {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.EncryptionConfig{
		Enabled:    true,
		UserSecret: "test-secret-12345",
	}

	service := NewService(cfg, log)
	return service
}

func TestService_NewService(t *testing.T) {
	service := setupTestService(t)

	if service == nil {
		t.Fatal("NewService returned nil")
	}

	if service.Name() != "encryption-service" {
		t.Errorf("Expected service name 'encryption-service', got %s", service.Name())
	}
}

func TestService_StartStop(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify key is derived
	keyHash := service.GetKeyHash()
	if keyHash == "" {
		t.Error("Expected key hash to be set after Start()")
	}

	err = service.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestService_Start_Disabled(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.EncryptionConfig{
		Enabled: false,
	}

	service := NewService(cfg, log)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start should succeed when disabled: %v", err)
	}
}

func TestService_EncryptDecryptData(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(ctx)

	// Test data
	plaintext := []byte("test data for encryption")

	// Encrypt
	ciphertext, err := service.EncryptData(plaintext)
	if err != nil {
		t.Fatalf("EncryptData failed: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("Expected ciphertext to be non-empty")
	}

	// Ciphertext should be different from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Error("Ciphertext should be different from plaintext")
	}

	// Decrypt
	decrypted, err := service.DecryptData(ciphertext)
	if err != nil {
		t.Fatalf("DecryptData failed: %v", err)
	}

	// Decrypted should match original
	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypted data doesn't match original: got %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestService_EncryptDecryptClip(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(ctx)

	// Create a test clip file
	tmpDir := t.TempDir()
	clipPath := filepath.Join(tmpDir, "test-clip.mp4")
	testData := []byte("fake video clip data")
	if err := os.WriteFile(clipPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test clip: %v", err)
	}

	// Encrypt clip
	encryptedPath, err := service.EncryptClip(ctx, clipPath)
	if err != nil {
		t.Fatalf("EncryptClip failed: %v", err)
	}

	// Verify encrypted file exists
	if _, err := os.Stat(encryptedPath); os.IsNotExist(err) {
		t.Fatal("Encrypted file was not created")
	}

	// Verify encrypted file is different from original
	encryptedData, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("Failed to read encrypted file: %v", err)
	}

	if string(encryptedData) == string(testData) {
		t.Error("Encrypted data should be different from original")
	}

	// Decrypt clip
	decryptedData, err := service.DecryptClip(ctx, encryptedPath)
	if err != nil {
		t.Fatalf("DecryptClip failed: %v", err)
	}

	// Verify decrypted data matches original
	if string(decryptedData) != string(testData) {
		t.Errorf("Decrypted data doesn't match original: got %s, want %s", string(decryptedData), string(testData))
	}
}

func TestService_EncryptClip_Disabled(t *testing.T) {
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.EncryptionConfig{
		Enabled: false,
	}

	service := NewService(cfg, log)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(ctx)

	// Create a test clip file
	tmpDir := t.TempDir()
	clipPath := filepath.Join(tmpDir, "test-clip.mp4")
	testData := []byte("test data")
	if err := os.WriteFile(clipPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test clip: %v", err)
	}

	// Encrypt clip (should return original path when disabled)
	encryptedPath, err := service.EncryptClip(ctx, clipPath)
	if err != nil {
		t.Fatalf("EncryptClip failed: %v", err)
	}

	// Should return original path when encryption is disabled
	if encryptedPath != clipPath {
		t.Errorf("Expected original path when disabled, got %s", encryptedPath)
	}
}

func TestService_GetEncryptionMetadata(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(ctx)

	metadata := service.GetEncryptionMetadata()

	if metadata == nil {
		t.Fatal("Expected encryption metadata, got nil")
	}

	if metadata.Algorithm != "AES-256-GCM" {
		t.Errorf("Expected algorithm 'AES-256-GCM', got %s", metadata.Algorithm)
	}

	if metadata.KeyDerivation != "Argon2id" {
		t.Errorf("Expected key derivation 'Argon2id', got %s", metadata.KeyDerivation)
	}

	if metadata.KeyHash == "" {
		t.Error("Expected key hash to be set")
	}

	if metadata.Salt == "" {
		t.Error("Expected salt to be set")
	}
}

func TestService_GetKeyHash(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	// Before start, key hash should be empty
	keyHash := service.GetKeyHash()
	if keyHash != "" {
		t.Error("Expected key hash to be empty before Start()")
	}

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(ctx)

	// After start, key hash should be set
	keyHash = service.GetKeyHash()
	if keyHash == "" {
		t.Error("Expected key hash to be set after Start()")
	}
}

func TestService_SaveLoadSalt(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop(ctx)

	// Get initial salt
	initialMetadata := service.GetEncryptionMetadata()
	initialSalt := initialMetadata.Salt

	// Save salt to file
	tmpDir := t.TempDir()
	saltPath := filepath.Join(tmpDir, "salt.txt")
	if err := service.SaveSaltToFile(saltPath); err != nil {
		t.Fatalf("SaveSaltToFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(saltPath); os.IsNotExist(err) {
		t.Fatal("Salt file was not created")
	}

	// Create a new service with salt path configured
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg := &config.EncryptionConfig{
		Enabled:    true,
		UserSecret: "test-secret-12345",
		SaltPath:   saltPath,
	}

	newService := NewService(cfg, log)

	// Start new service (will load salt from file automatically)
	err = newService.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer newService.Stop(ctx)

	// Verify salt matches
	newMetadata := newService.GetEncryptionMetadata()
	if newMetadata.Salt != initialSalt {
		t.Errorf("Loaded salt doesn't match original: got %s, want %s", newMetadata.Salt, initialSalt)
	}
}

func TestService_KeyDerivation(t *testing.T) {
	// Test that same secret + salt produces same key
	log, _ := logger.New(logger.LogConfig{
		Level:  "debug",
		Format: "text",
		Output: "stdout",
	})

	cfg1 := &config.EncryptionConfig{
		Enabled:    true,
		UserSecret: "test-secret",
		Salt:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	service1 := NewService(cfg1, log)
	ctx := context.Background()
	if err := service1.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service1.Stop(ctx)

	cfg2 := &config.EncryptionConfig{
		Enabled:    true,
		UserSecret: "test-secret",
		Salt:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	service2 := NewService(cfg2, log)
	if err := service2.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service2.Stop(ctx)

	// Both services should produce the same key hash
	hash1 := service1.GetKeyHash()
	hash2 := service2.GetKeyHash()

	if hash1 != hash2 {
		t.Errorf("Same secret should produce same key hash: got %s and %s", hash1, hash2)
	}

	// Test encryption/decryption with both services
	testData := []byte("test data")
	ciphertext1, err := service1.EncryptData(testData)
	if err != nil {
		t.Fatalf("EncryptData failed: %v", err)
	}

	// Service 2 should be able to decrypt data encrypted by service 1
	decrypted, err := service2.DecryptData(ciphertext1)
	if err != nil {
		t.Fatalf("DecryptData failed: %v", err)
	}

	if string(decrypted) != string(testData) {
		t.Errorf("Decrypted data doesn't match: got %s, want %s", string(decrypted), string(testData))
	}
}

