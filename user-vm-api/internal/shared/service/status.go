package service

import (
	"sync"
	"time"
)

// Status represents the state of a service
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusError    Status = "error"
)

// ServiceStatus tracks the status of a service
type ServiceStatus struct {
	Name      string
	Status    Status
	StartedAt time.Time
	Error     error
	mu        sync.RWMutex
}

// NewServiceStatus creates a new service status tracker
func NewServiceStatus(name string) *ServiceStatus {
	return &ServiceStatus{
		Name:   name,
		Status: StatusStopped,
	}
}

// SetStatus sets the service status
func (ss *ServiceStatus) SetStatus(status Status) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.Status = status
	if status == StatusRunning {
		ss.StartedAt = time.Now()
		ss.Error = nil
	}
}

// SetError sets the service error status
func (ss *ServiceStatus) SetError(err error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.Status = StatusError
	ss.Error = err
}

// GetStatus returns the current status
func (ss *ServiceStatus) GetStatus() Status {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.Status
}

// GetError returns the current error
func (ss *ServiceStatus) GetError() error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.Error
}

// IsRunning returns true if the service is running
func (ss *ServiceStatus) IsRunning() bool {
	return ss.GetStatus() == StatusRunning
}

// GetUptime returns the uptime of the service
func (ss *ServiceStatus) GetUptime() time.Duration {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.Status == StatusRunning && !ss.StartedAt.IsZero() {
		return time.Since(ss.StartedAt)
	}
	return 0
}

