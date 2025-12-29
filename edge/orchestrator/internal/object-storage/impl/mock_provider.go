package impl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/object-storage/types"
)

// mockObjectProvider is a mock implementation of ObjectStorageProvider for testing
type mockObjectProvider struct {
	objects map[string]*mockObject
	mu      sync.RWMutex
}

type mockObject struct {
	key          string
	data         []byte
	size         int64
	contentType  string
	metadata     map[string]string
	lastModified int64
	etag         string
}

func newMockObjectProvider() *mockObjectProvider {
	return &mockObjectProvider{
		objects: make(map[string]*mockObject),
	}
}

func (m *mockObjectProvider) StoreObject(ctx context.Context, key string, r io.Reader, size int64, contentType string, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// Calculate SHA-256 hash
	hash := sha256.Sum256(data)
	hashStr := fmt.Sprintf("%x", hash)

	// Use hash from metadata if provided, otherwise calculate it
	if hashFromMeta, ok := metadata["hash"]; ok && hashFromMeta != "" {
		hashStr = hashFromMeta
	} else {
		// Store calculated hash in metadata
		if metadata == nil {
			metadata = make(map[string]string)
		}
		metadata["hash"] = hashStr
	}

	m.objects[key] = &mockObject{
		key:          key,
		data:         data,
		size:         size,
		contentType:  contentType,
		metadata:     metadata,
		lastModified: time.Now().Unix(),
		etag:         hashStr,
	}
	return nil
}

func (m *mockObjectProvider) LoadObject(ctx context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	obj, exists := m.objects[key]
	if !exists {
		return nil, types.ErrObjectNotFound
	}

	return io.NopCloser(bytes.NewReader(obj.data)), nil
}

func (m *mockObjectProvider) DeleteObject(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.objects, key)
	return nil
}

func (m *mockObjectProvider) ListObjects(ctx context.Context, prefix string) ([]types.ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []types.ObjectInfo
	for key, obj := range m.objects {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			results = append(results, types.ObjectInfo{
				Key:          key,
				Size:         obj.size,
				ContentType:  obj.contentType,
				LastModified: obj.lastModified,
				ETag:         obj.etag,
			})
		}
	}
	return results, nil
}

func (m *mockObjectProvider) GetObjectMetadata(ctx context.Context, key string) (*types.ObjectMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	obj, exists := m.objects[key]
	if !exists {
		return nil, types.ErrObjectNotFound
	}

	metadata := &types.ObjectMetadata{
		Key:         key,
		Size:        obj.size,
		ContentType: obj.contentType,
		Hash:        obj.etag,
		CreatedAt:   time.Unix(obj.lastModified, 0),
		Metadata:    obj.metadata,
	}

	// Extract created_at from metadata if present
	if createdStr, ok := obj.metadata["created_at"]; ok {
		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			metadata.CreatedAt = t
		}
	}

	// Extract upload_completed_at from metadata if present
	if uploadStr, ok := obj.metadata["upload_completed_at"]; ok {
		if t, err := time.Parse(time.RFC3339, uploadStr); err == nil {
			metadata.UploadCompletedAt = &t
		}
	}

	// Extract vm_ack_at from metadata if present
	if vmAckStr, ok := obj.metadata["vm_ack_at"]; ok {
		if t, err := time.Parse(time.RFC3339, vmAckStr); err == nil {
			metadata.VMAckAt = &t
		}
	}

	if deviceID, ok := obj.metadata["device_id"]; ok {
		metadata.DeviceID = types.DeviceID(deviceID)
	}
	if deviceType, ok := obj.metadata["device_type"]; ok {
		metadata.DeviceType = types.DeviceType(deviceType)
	}

	return metadata, nil
}

func (m *mockObjectProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockObjectProvider) Close() error {
	return nil
}

