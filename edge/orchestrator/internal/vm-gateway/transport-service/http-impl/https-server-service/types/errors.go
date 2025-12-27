package types

import "errors"

// HTTPServer errors
var (
	// ErrDeviceIDRequired is returned when device_id is missing from model deployment metadata.
	// Models are trained on specific device datasets and must be associated with a device.
	ErrDeviceIDRequired = errors.New("device_id is required - models are trained on specific device datasets")
)

