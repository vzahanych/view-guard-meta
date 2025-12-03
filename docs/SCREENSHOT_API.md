# Screenshot Management API Documentation

This document describes the REST API endpoints for managing labeled screenshots in the View Guard Edge Orchestrator.

## Base URL

All endpoints are relative to the base URL:
```
http://localhost:8080/api
```

## Authentication

Currently, the API does not require authentication for the PoC. Future versions will implement token-based authentication.

## Endpoints

### 1. Save Screenshot

Create a new labeled screenshot from image data.

**Endpoint**: `POST /api/screenshots`

**Request Body**:
```json
{
  "camera_id": "camera-1",
  "label": "normal",
  "custom_label": "optional-custom-label",
  "description": "Optional description of the screenshot",
  "image_data": "base64-encoded-image-data",
  "created_by": "user@example.com"
}
```

**Parameters**:
- `camera_id` (string, required): ID of the camera that captured the screenshot
- `label` (string, required): Label type - one of: `"normal"`, `"threat"`, `"abnormal"`, `"custom"`
- `custom_label` (string, optional): Custom label name (required if `label` is `"custom"`)
- `description` (string, optional): Human-readable description
- `image_data` (string, required): Base64-encoded image data (JPEG or PNG)
- `created_by` (string, optional): User identifier for audit trail

**Response** (201 Created):
```json
{
  "id": "screenshot-uuid",
  "camera_id": "camera-1",
  "file_path": "/path/to/screenshot.jpg",
  "label": "normal",
  "custom_label": null,
  "description": "Optional description",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z",
  "dataset_status": {
    "label_counts": {
      "normal": 25,
      "threat": 2,
      "abnormal": 1
    },
    "labeled_snapshot_count": 25,
    "required_snapshot_count": 50,
    "snapshot_required": true,
    "last_synced": "2024-01-15T10:30:00Z"
  }
}
```

**Error Responses**:
- `400 Bad Request`: Invalid request data or image format
- `500 Internal Server Error`: Failed to save screenshot

**Example**:
```bash
curl -X POST http://localhost:8080/api/screenshots \
  -H "Content-Type: application/json" \
  -d '{
    "camera_id": "camera-1",
    "label": "normal",
    "description": "Normal parking lot scene",
    "image_data": "iVBORw0KGgoAAAANSUhEUgAA...",
    "created_by": "admin@example.com"
  }'
```

---

### 2. List Screenshots

Get a list of screenshots with optional filtering.

**Endpoint**: `GET /api/screenshots`

**Query Parameters**:
- `camera_id` (string, optional): Filter by camera ID
- `label` (string, optional): Filter by label type
- `limit` (integer, optional): Maximum number of results (default: 100)
- `offset` (integer, optional): Pagination offset (default: 0)

**Response** (200 OK):
```json
{
  "screenshots": [
    {
      "id": "screenshot-uuid-1",
      "camera_id": "camera-1",
      "file_path": "/path/to/screenshot1.jpg",
      "label": "normal",
      "custom_label": null,
      "description": "Description",
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T10:30:00Z",
      "created_by": "admin@example.com"
    },
    {
      "id": "screenshot-uuid-2",
      "camera_id": "camera-1",
      "file_path": "/path/to/screenshot2.jpg",
      "label": "threat",
      "custom_label": null,
      "description": "Threat detected",
      "created_at": "2024-01-15T11:00:00Z",
      "updated_at": "2024-01-15T11:00:00Z",
      "created_by": "admin@example.com"
    }
  ],
  "count": 2,
  "total": 150,
  "limit": 100,
  "offset": 0
}
```

**Example**:
```bash
curl "http://localhost:8080/api/screenshots?camera_id=camera-1&label=normal&limit=10"
```

---

### 3. Get Screenshot

Get details of a specific screenshot.

**Endpoint**: `GET /api/screenshots/{id}`

**Path Parameters**:
- `id` (string, required): Screenshot ID

**Response** (200 OK):
```json
{
  "id": "screenshot-uuid",
  "camera_id": "camera-1",
  "file_path": "/path/to/screenshot.jpg",
  "label": "normal",
  "custom_label": null,
  "description": "Description",
  "metadata": {
    "width": 1920,
    "height": 1080,
    "original_format": "jpeg",
    "original_size_bytes": 245678,
    "processed_size_bytes": 189234,
    "compression_ratio": 0.77
  },
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z",
  "created_by": "admin@example.com"
}
```

**Error Responses**:
- `404 Not Found`: Screenshot not found

**Example**:
```bash
curl http://localhost:8080/api/screenshots/screenshot-uuid
```

---

### 4. Get Screenshot Image

Get the actual image file for a screenshot.

**Endpoint**: `GET /api/screenshots/{id}/image`

**Path Parameters**:
- `id` (string, required): Screenshot ID

**Response** (200 OK):
- Content-Type: `image/jpeg`
- Body: Binary image data

**Error Responses**:
- `404 Not Found`: Screenshot not found
- `500 Internal Server Error`: Failed to read image file

**Example**:
```bash
curl http://localhost:8080/api/screenshots/screenshot-uuid/image -o screenshot.jpg
```

---

### 5. Update Screenshot

Update the label, description, or metadata of a screenshot.

**Endpoint**: `PUT /api/screenshots/{id}`

**Path Parameters**:
- `id` (string, required): Screenshot ID

**Request Body**:
```json
{
  "label": "threat",
  "custom_label": "optional-custom-label",
  "description": "Updated description",
  "metadata": {
    "custom_field": "value"
  }
}
```

**Parameters** (all optional):
- `label` (string): New label type
- `custom_label` (string): New custom label name
- `description` (string): New description
- `metadata` (object): Additional metadata fields

**Response** (200 OK):
```json
{
  "id": "screenshot-uuid",
  "camera_id": "camera-1",
  "file_path": "/path/to/screenshot.jpg",
  "label": "threat",
  "custom_label": null,
  "description": "Updated description",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T12:00:00Z",
  "dataset_status": {
    "label_counts": {
      "normal": 24,
      "threat": 3,
      "abnormal": 1
    },
    "labeled_snapshot_count": 24,
    "required_snapshot_count": 50,
    "snapshot_required": true,
    "last_synced": "2024-01-15T12:00:00Z"
  }
}
```

**Error Responses**:
- `400 Bad Request`: Invalid request data
- `404 Not Found`: Screenshot not found
- `500 Internal Server Error`: Failed to update screenshot

**Example**:
```bash
curl -X PUT http://localhost:8080/api/screenshots/screenshot-uuid \
  -H "Content-Type: application/json" \
  -d '{
    "label": "threat",
    "description": "Updated description"
  }'
```

---

### 6. Delete Screenshot

Delete a screenshot and its associated image file.

**Endpoint**: `DELETE /api/screenshots/{id}`

**Path Parameters**:
- `id` (string, required): Screenshot ID

**Response** (204 No Content): Empty response body

**Error Responses**:
- `404 Not Found`: Screenshot not found
- `500 Internal Server Error`: Failed to delete screenshot

**Example**:
```bash
curl -X DELETE http://localhost:8080/api/screenshots/screenshot-uuid
```

---

### 7. Get Dataset Status (Edge)

Get dataset progress status for a camera as computed on the Edge.

**Endpoints**:

- `GET /api/cameras/{camera_id}/dataset`
- `GET /api/cameras/{camera_id}/dataset-status` (alias for the same handler)

**Path Parameters**:
- `camera_id` (string, required): Camera ID

**Response** (200 OK):
```json
{
  "camera_id": "camera-1",
  "dataset_status": {
    "label_counts": {
      "normal": 45,
      "threat": 5,
      "abnormal": 2,
      "custom": 1
    },
    "labeled_snapshot_count": 45,
    "required_snapshot_count": 50,
    "snapshot_required": true,
    "last_synced": "2024-01-15T10:30:00Z"
  }
}
```

**Notes**:
- `required_snapshot_count` is driven by `min_normal_snapshots` in Edge config (default: 50).
- Only `"normal"` labeled screenshots contribute to `labeled_snapshot_count` and `snapshot_required`.

**Error Responses**:
- `404 Not Found`: Camera not found
- `500 Internal Server Error`: Failed to get dataset status

**Example**:
```bash
curl http://localhost:8080/api/cameras/camera-1/dataset
```

---

### 8. Sync Dataset Status to VM (manual, per camera)

Trigger a manual dataset sync for a specific camera. This endpoint validates that the Edge dataset is ready (≥ required number of `"normal"` snapshots), packages all labeled screenshots into a tar.gz archive, uploads the dataset to the VM, and updates the training eligibility status.

**Endpoint**: `POST /api/cameras/{camera_id}/dataset/sync`

**Path Parameters**:
- `camera_id` (string, required): Camera ID

**Response (Success)** (200 OK):
```json
{
  "camera_id": "camera-1",
  "dataset_id": "43f59351-789f-4390-9f61-3963e1e171ac",
  "dataset_status": {
    "label_counts": {
      "normal": 50,
      "threat": 5,
      "abnormal": 2
    },
    "labeled_snapshot_count": 50,
    "required_snapshot_count": 50,
    "snapshot_required": false,
    "last_synced": "2024-01-15T10:30:00Z"
  },
  "message": "Dataset synced successfully to VM"
}
```

**Response (Not Ready)** (409 Conflict):
```json
{
  "error": "Dataset not ready for training (snapshot_required=true)",
  "camera_id": "camera-1",
  "dataset_status": {
    "label_counts": {
      "normal": 25
    },
    "labeled_snapshot_count": 25,
    "required_snapshot_count": 50,
    "snapshot_required": true,
    "last_synced": "2024-01-15T10:00:00Z"
  },
  "required_snapshots": 50
}
```

**Response (Connection Error)** (503 Service Unavailable):
```json
{
  "error": "Connection unavailable: failed to connect to VM",
  "camera_id": "camera-1",
  "message": "Unable to sync dataset to VM. Please check network connectivity and VM status."
}
```

**Response (Upload Failure)** (500 Internal Server Error):
```json
{
  "error": "Dataset upload failed: <error details>",
  "camera_id": "camera-1",
  "message": "Failed to upload dataset to VM after retries"
}
```

**Process Flow**:
1. Validates dataset readiness (checks `snapshot_required` status)
2. Syncs camera capabilities to VM via gRPC (pre-upload)
3. Collects all labeled screenshots for the camera
4. Packages screenshots into a tar.gz archive with metadata and manifest
5. Calculates SHA-256 checksum of the archive
6. Uploads archive to VM via HTTP multipart form (`POST /api/datasets/upload`)
7. VM extracts archive, verifies checksum, and stores dataset
8. Updates training eligibility status to `ready_for_training` on VM
9. Syncs camera capabilities to VM again (post-upload)
10. Cleans up temporary archive file on Edge

**Error Responses**:
- `400 Bad Request`: Camera ID missing or invalid
- `409 Conflict`: Dataset not yet ready (`snapshot_required=true`)
- `500 Internal Server Error`: Failed to package or upload dataset
- `503 Service Unavailable`: VM connection unavailable

**Example**:
```bash
curl -X POST http://localhost:8080/api/cameras/camera-1/dataset/sync
```

**Notes**:
- The dataset archive includes:
  - `metadata.json`: Dataset metadata (edge_id, camera_id, total_snapshots, label_counts, synced_at, checksum)
  - `manifest.json`: List of all screenshots with labels and file paths
  - `screenshots/`: Directory containing all screenshot image files (`.jpg`)
- Duplicate uploads (same edge_id, camera_id, and checksum) are detected and skipped on the VM side
- The archive is automatically cleaned up after successful upload
- Upload includes retry logic with exponential backoff (up to 3 attempts)

---

### 9. Export Dataset

Export labeled screenshots as a dataset archive.

**Endpoint**: `POST /api/screenshots/export`

**Request Body**:
```json
{
  "camera_id": "camera-1",
  "label": "normal",
  "format": "zip"
}
```

**Parameters** (all optional):
- `camera_id` (string): Filter by camera ID
- `label` (string): Filter by label type
- `format` (string): Export format - `"zip"` (default)

**Response** (200 OK):
- Content-Type: `application/zip`
- Content-Disposition: `attachment; filename="dataset-export-YYYY-MM-DD.zip"`
- Body: ZIP archive containing screenshots and metadata

**Error Responses**:
- `400 Bad Request`: Invalid request parameters
- `500 Internal Server Error`: Failed to export dataset

**Example**:
```bash
curl -X POST http://localhost:8080/api/screenshots/export \
  -H "Content-Type: application/json" \
  -d '{
    "camera_id": "camera-1",
    "label": "normal"
  }' \
  -o dataset-export.zip
```

---

## Error Response Format

All error responses follow this format:

```json
{
  "error": "Error message describing what went wrong"
}
```

## Rate Limiting

Currently, there are no rate limits. Future versions may implement rate limiting based on authentication and subscription tier.

## Data Formats

### Image Data

- **Input Format**: Base64-encoded JPEG or PNG
- **Storage Format**: JPEG (PNG images are automatically converted)
- **Quality**: 85% JPEG quality (configurable)
- **Optimization**: Automatic compression and format conversion

### Timestamps

All timestamps are in ISO 8601 format (RFC3339):
```
2024-01-15T10:30:00Z
```

### Labels

Valid label values:
- `"normal"`: Normal/baseline scenes
- `"threat"`: Threat detection scenes
- `"abnormal"`: Anomaly detection scenes
- `"custom"`: Custom labeled scenes (requires `custom_label` field)

## Best Practices

1. **Batch Operations**: When saving multiple screenshots, use individual API calls rather than batching
2. **Image Size**: Keep image sizes reasonable (< 10MB recommended)
3. **Error Handling**: Always check response status codes and handle errors appropriately
4. **Pagination**: Use `limit` and `offset` for large result sets
5. **Audit Trail**: Include `created_by` field for audit purposes
6. **Metadata**: Use descriptions to provide context for screenshots

---

## Dataset Storage Structure (VM)

When datasets are uploaded from Edge to VM, they are organized in a structured directory hierarchy:

### Directory Structure

```
/app/data/datasets/
├── {edge_id}/
│   ├── {camera_id}/
│   │   ├── {dataset_id}/
│   │   │   ├── metadata.json          # Dataset metadata
│   │   │   ├── manifest.json          # Screenshot manifest (optional)
│   │   │   └── screenshots/
│   │   │       ├── {screenshot_id_1}.jpg
│   │   │       ├── {screenshot_id_2}.jpg
│   │   │       └── ...
│   │   └── {dataset_id_2}/
│   │       └── ...
│   └── {edge_id_2}/
│       └── ...
```

### Path Format

```
/app/data/datasets/{edge_id}/{camera_id}/{dataset_id}/
```

Where:
- `{edge_id}`: Unique identifier for the Edge appliance
- `{camera_id}`: Camera identifier
- `{dataset_id}`: UUID generated by VM when dataset is received

### Dataset Contents

#### metadata.json

Contains dataset metadata:

```json
{
  "edge_id": "edge-123",
  "camera_id": "camera-1",
  "total_snapshots": 50,
  "label_counts": {
    "normal": 45,
    "threat": 3,
    "abnormal": 2
  },
  "synced_at": "2024-01-15T10:30:00Z",
  "checksum": "c6290d588a08e5dff6ca0b66a49ee32c97cc21dfb26a5cc8a23d706431f0b7cf"
}
```

#### manifest.json

Contains list of all screenshots in the dataset:

```json
{
  "camera_id": "camera-1",
  "synced_at": "2024-01-15T10:30:00Z",
  "screenshots": [
    {
      "screenshot_id": "screenshot-uuid-1",
      "label": "normal",
      "custom_label": "",
      "description": "Normal scene",
      "created_at": "2024-01-15T09:00:00Z",
      "file_path": "screenshots/screenshot-uuid-1.jpg"
    },
    ...
  ]
}
```

#### screenshots/

Directory containing all screenshot image files in JPEG format. Files are named using the screenshot ID: `{screenshot_id}.jpg`.

### Database Storage

Dataset metadata is also stored in the `training_datasets` table:

- `dataset_id`: UUID identifier
- `name`: Dataset name (derived from edge_id and camera_id)
- `edge_id`: Edge appliance identifier
- `dataset_dir_path`: Filesystem path to dataset directory
- `label_counts`: JSON object with label distribution
- `total_images`: Total number of screenshots
- `status`: Dataset status (`ready`, `processing`, etc.)
- `checksum`: SHA-256 checksum for duplicate detection
- `created_at`, `updated_at`: Timestamps

### Training Eligibility Status

After successful dataset upload, the VM updates the `edge_camera_status` table:

- `training_eligibility_status`: Set to `ready_for_training`
- `dataset_id`: Linked to the uploaded dataset

---

## Troubleshooting Dataset Sync

### Common Issues and Solutions

#### 1. Dataset Not Ready (409 Conflict)

**Symptom**: API returns `409 Conflict` with message "Dataset not ready for training (snapshot_required=true)"

**Cause**: Camera has fewer than the required number of `"normal"` labeled snapshots (default: 50)

**Solution**:
- Capture and label more screenshots with `"normal"` label
- Check current snapshot count: `GET /api/cameras/{camera_id}/dataset`
- Ensure screenshots are labeled correctly (only `"normal"` labeled screenshots count toward the requirement)

#### 2. Connection Unavailable (503 Service Unavailable)

**Symptom**: API returns `503 Service Unavailable` with message "Connection unavailable"

**Causes**:
- VM is not running or not accessible
- WireGuard tunnel is not established
- Network connectivity issues between Edge and VM
- VM endpoint configuration is incorrect

**Solutions**:
- Verify VM is running: Check VM container/service status
- Check WireGuard tunnel status: Verify Edge can reach VM via tunnel
- Verify network connectivity: Test connection to VM endpoint
- Check Edge configuration: Verify `edge.wireguard.kvm_endpoint` is correct
- Check firewall rules: Ensure WireGuard port (default: 51820) is open

#### 3. Upload Failure (500 Internal Server Error)

**Symptom**: API returns `500 Internal Server Error` with upload failure message

**Causes**:
- VM server error during dataset reception
- Archive corruption or invalid format
- Checksum mismatch
- Disk space insufficient on VM
- Database error on VM

**Solutions**:
- Check VM logs for detailed error messages
- Verify archive integrity: Check if archive was created correctly
- Check VM disk space: Ensure sufficient space for dataset storage
- Verify VM database is accessible and healthy
- Retry the sync operation (automatic retry logic handles transient errors)

#### 4. Duplicate Upload Detection

**Symptom**: Upload succeeds but returns existing dataset_id (no error, but dataset already exists)

**Cause**: Dataset with the same checksum (same content) was already uploaded

**Behavior**: This is expected and not an error. The VM detects duplicates by comparing checksums and returns the existing dataset ID instead of creating a duplicate.

**Solution**: No action needed. The existing dataset is reused.

#### 5. Archive Cleanup Failure

**Symptom**: Warning in logs about failed archive cleanup after upload

**Cause**: Temporary archive file could not be deleted (permissions, file locked, etc.)

**Impact**: Minimal - archive is in temporary directory and will be cleaned up eventually

**Solution**: Check file permissions on Edge export directory, ensure sufficient disk space

#### 6. Training Eligibility Not Updated

**Symptom**: Dataset uploads successfully but `training_eligibility_status` remains `needs_snapshots`

**Causes**:
- Capability store update failed (logged but upload still succeeds)
- Database constraint violation
- Camera not found in `edge_camera_status` table

**Solutions**:
- Check VM logs for capability store update errors
- Verify camera exists in `edge_camera_status` table
- Manually update training eligibility if needed
- Check database foreign key constraints

### Debugging Steps

1. **Check Edge Logs**:
   ```bash
   # View Edge orchestrator logs
   docker logs edge-orchestrator | grep -i "dataset\|sync"
   ```

2. **Check VM Logs**:
   ```bash
   # View VM API logs
   docker logs user-vm-api | grep -i "dataset\|upload"
   ```

3. **Verify Dataset Status**:
   ```bash
   # Check dataset status on Edge
   curl http://localhost:8080/api/cameras/{camera_id}/dataset | jq .
   ```

4. **Test VM Connectivity**:
   ```bash
   # From Edge, test connection to VM
   curl -v http://{vm_endpoint}/api/health
   ```

5. **Check Archive Contents** (if accessible):
   ```bash
   # Extract and inspect archive (on Edge, before cleanup)
   tar -tzf /path/to/dataset_*.tar.gz
   ```

6. **Verify Database State**:
   ```sql
   -- Check training_datasets table
   SELECT * FROM training_datasets WHERE edge_id = ? AND camera_id = ?;
   
   -- Check edge_camera_status
   SELECT * FROM edge_camera_status WHERE edge_id = ? AND camera_id = ?;
   ```

### Performance Considerations

- **Large Datasets**: Uploads may take several minutes for large datasets (100+ screenshots). The upload includes a 10-minute timeout.
- **Network Bandwidth**: Dataset archives are compressed (gzip), but large datasets still require significant bandwidth.
- **Retry Logic**: Automatic retries use exponential backoff (1s, 2s, 4s delays) for transient failures.
- **Concurrent Syncs**: Only one sync operation should be performed per camera at a time.

---

## Related Documentation

- [Screenshot User Guide](./SCREENSHOT_USER_GUIDE.md)
- [Screenshot Configuration Guide](./SCREENSHOT_CONFIGURATION.md)
- [Implementation Plan Phase 2](./IMPLEMENTATION_PLAN_PHASE2.md)
