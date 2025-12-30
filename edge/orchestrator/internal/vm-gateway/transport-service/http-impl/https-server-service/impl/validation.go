package impl

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"
)

// ValidationError represents a validation error with a field name and message.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

// ValidateString validates a string field.
func ValidateString(fieldName, value string, minLen, maxLen int, required bool) error {
	if required && value == "" {
		return &ValidationError{Field: fieldName, Message: "is required"}
	}
	if value == "" {
		return nil // Empty optional field is valid
	}
	if minLen > 0 && len(value) < minLen {
		return &ValidationError{Field: fieldName, Message: fmt.Sprintf("must be at least %d characters", minLen)}
	}
	if maxLen > 0 && len(value) > maxLen {
		return &ValidationError{Field: fieldName, Message: fmt.Sprintf("must be at most %d characters", maxLen)}
	}
	if !utf8.ValidString(value) {
		return &ValidationError{Field: fieldName, Message: "contains invalid UTF-8 characters"}
	}
	return nil
}

// ValidateDeviceID validates a device ID.
func ValidateDeviceID(deviceID string) error {
	if err := ValidateString("device_id", deviceID, 1, 255, true); err != nil {
		return err
	}
	// Device ID should be alphanumeric with hyphens and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, deviceID)
	if !matched {
		return &ValidationError{Field: "device_id", Message: "must contain only alphanumeric characters, hyphens, and underscores"}
	}
	return nil
}

// ValidateModelID validates a model ID.
func ValidateModelID(modelID string) error {
	if err := ValidateString("model_id", modelID, 1, 255, true); err != nil {
		return err
	}
	// Model ID should be alphanumeric with hyphens and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, modelID)
	if !matched {
		return &ValidationError{Field: "model_id", Message: "must contain only alphanumeric characters, hyphens, and underscores"}
	}
	return nil
}

// ValidateDeploymentID validates a deployment ID.
func ValidateDeploymentID(deploymentID string) error {
	if err := ValidateString("deployment_id", deploymentID, 1, 255, false); err != nil {
		return err
	}
	if deploymentID == "" {
		return nil // Optional field
	}
	// Deployment ID should be alphanumeric with hyphens and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, deploymentID)
	if !matched {
		return &ValidationError{Field: "deployment_id", Message: "must contain only alphanumeric characters, hyphens, and underscores"}
	}
	return nil
}

// ValidateLabel validates a data unit label.
func ValidateLabel(label string) error {
	validLabels := map[string]bool{
		"normal":   true,
		"threat":   true,
		"abnormal": true,
		"custom":   true,
	}
	if !validLabels[label] {
		return &ValidationError{Field: "label", Message: "must be one of: normal, threat, abnormal, custom"}
	}
	return nil
}

// ValidateCustomLabel validates a custom label.
func ValidateCustomLabel(customLabel string, required bool) error {
	if required && customLabel == "" {
		return &ValidationError{Field: "custom_label", Message: "is required when label is 'custom'"}
	}
	if customLabel != "" {
		if err := ValidateString("custom_label", customLabel, 1, 255, false); err != nil {
			return err
		}
		// Custom label should not contain control characters or special injection characters
		if strings.ContainsAny(customLabel, "\x00\n\r\t") {
			return &ValidationError{Field: "custom_label", Message: "must not contain control characters"}
		}
	}
	return nil
}

// ValidateCount validates a count value.
func ValidateCount(count int32, min, max int32) error {
	if count < min {
		return &ValidationError{Field: "count", Message: fmt.Sprintf("must be at least %d", min)}
	}
	if max > 0 && count > max {
		return &ValidationError{Field: "count", Message: fmt.Sprintf("must be at most %d", max)}
	}
	return nil
}

// ValidateJSON validates that a string is valid JSON.
func ValidateJSON(fieldName, jsonStr string, required bool) error {
	if required && jsonStr == "" {
		return &ValidationError{Field: fieldName, Message: "is required"}
	}
	if jsonStr == "" {
		return nil // Empty optional field is valid
	}
	// Check for basic JSON structure (starts with { or [)
	trimmed := strings.TrimSpace(jsonStr)
	if trimmed == "" {
		return nil
	}
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return &ValidationError{Field: fieldName, Message: "must be valid JSON"}
	}
	// Check for reasonable size (prevent DoS)
	if len(jsonStr) > 10*1024*1024 { // 10MB max
		return &ValidationError{Field: fieldName, Message: "must be less than 10MB"}
	}
	return nil
}

// ValidateVersion validates a version string.
func ValidateVersion(version string, required bool) error {
	if required && version == "" {
		return &ValidationError{Field: "version", Message: "is required"}
	}
	if version == "" {
		return nil // Optional field
	}
	if err := ValidateString("version", version, 1, 100, false); err != nil {
		return err
	}
	// Version should follow semantic versioning or be alphanumeric
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, version)
	if !matched {
		return &ValidationError{Field: "version", Message: "must contain only alphanumeric characters, dots, hyphens, and underscores"}
	}
	return nil
}

// ValidateModelType validates a model type.
func ValidateModelType(modelType string, required bool) error {
	if required && modelType == "" {
		return &ValidationError{Field: "model_type", Message: "is required"}
	}
	if modelType == "" {
		return nil // Optional field
	}
	if err := ValidateString("model_type", modelType, 1, 100, false); err != nil {
		return err
	}
	// Model type should be alphanumeric with hyphens and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, modelType)
	if !matched {
		return &ValidationError{Field: "model_type", Message: "must contain only alphanumeric characters, hyphens, and underscores"}
	}
	return nil
}

// ValidateFramework validates a framework name.
func ValidateFramework(framework string, required bool) error {
	if required && framework == "" {
		return &ValidationError{Field: "framework", Message: "is required"}
	}
	if framework == "" {
		return nil // Optional field
	}
	if err := ValidateString("framework", framework, 1, 100, false); err != nil {
		return err
	}
	// Framework should be alphanumeric with hyphens and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, framework)
	if !matched {
		return &ValidationError{Field: "framework", Message: "must contain only alphanumeric characters, hyphens, and underscores"}
	}
	return nil
}

// ValidateInputShape validates an input shape array.
func ValidateInputShape(inputShape []int) error {
	if len(inputShape) == 0 {
		return nil // Optional field
	}
	if len(inputShape) > 10 {
		return &ValidationError{Field: "input_shape", Message: "must have at most 10 dimensions"}
	}
	for i, dim := range inputShape {
		if dim <= 0 {
			return &ValidationError{Field: "input_shape", Message: fmt.Sprintf("dimension %d must be positive", i)}
		}
		if dim > 100000 {
			return &ValidationError{Field: "input_shape", Message: fmt.Sprintf("dimension %d must be at most 100000", i)}
		}
	}
	return nil
}

// LimitRequestBody wraps the request body with MaxBytesReader to limit the maximum
// size of the request body. This works for both Content-Length and chunked requests.
// Returns an error if the body exceeds maxBytes.
func LimitRequestBody(r *http.Request, maxBytes int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	return nil
}

// ValidateRequestSize validates the request body size.
// Deprecated: Use LimitRequestBody with MaxBytesReader instead, which handles both
// Content-Length and chunked requests.
func ValidateRequestSize(contentLength int64, maxSize int64) error {
	if contentLength > maxSize {
		return &ValidationError{Field: "request_body", Message: fmt.Sprintf("exceeds maximum size of %d bytes", maxSize)}
	}
	return nil
}

// SanitizeString sanitizes a string to prevent injection attacks.
func SanitizeString(value string) string {
	// Remove null bytes
	value = strings.ReplaceAll(value, "\x00", "")
	// Remove control characters except newline and tab
	var sanitized strings.Builder
	for _, r := range value {
		if r == '\n' || r == '\t' || (r >= 32 && r != 127) || r > 127 {
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}

// handleValidationError handles validation errors by sending an HTTP 400 response and logging.
func (s *HTTPServer) handleValidationError(w http.ResponseWriter, err error, logger *zap.Logger) {
	if validationErr, ok := err.(*ValidationError); ok {
		logger.Warn("Validation error",
			zap.String("field", validationErr.Field),
			zap.String("message", validationErr.Message))
		s.sendErrorResponse(w, http.StatusBadRequest, validationErr.Error())
	} else {
		logger.Warn("Validation error", zap.Error(err))
		s.sendErrorResponse(w, http.StatusBadRequest, err.Error())
	}
}

