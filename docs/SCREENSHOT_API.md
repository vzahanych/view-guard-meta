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

Trigger a manual dataset status sync for a specific camera. This validates that the Edge dataset is ready (≥ required number of `"normal"` snapshots) and, in future phases, will be extended to push readiness to the VM.

**Endpoint**: `POST /api/cameras/{camera_id}/dataset/sync`

**Path Parameters**:
- `camera_id` (string, required): Camera ID

**Response (Ready)** (200 OK):
```json
{
  "camera_id": "camera-1",
  "dataset_synced": false,
  "dataset_status": {
    "label_counts": {
      "normal": 50
    },
    "labeled_snapshot_count": 50,
    "required_snapshot_count": 50,
    "snapshot_required": false,
    "last_synced": "2024-01-15T10:30:00Z"
  },
  "message": "Local dataset is ready; VM sync endpoint not yet implemented"
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

**Error Responses**:
- `400 Bad Request`: Camera ID missing or invalid
- `409 Conflict`: Dataset not yet ready (`snapshot_required=true`)
- `500 Internal Server Error`: Failed to compute dataset status

**Example**:
```bash
curl -X POST http://localhost:8080/api/cameras/camera-1/dataset/sync
```

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

## Related Documentation

- [Screenshot User Guide](./SCREENSHOT_USER_GUIDE.md)
- [Screenshot Configuration Guide](./SCREENSHOT_CONFIGURATION.md)
- [Implementation Plan Phase 2](./IMPLEMENTATION_PLAN_PHASE2.md)
