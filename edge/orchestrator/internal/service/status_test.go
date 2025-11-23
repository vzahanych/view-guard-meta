package service

import (
	"errors"
	"testing"
	"time"
)

func TestNewServiceStatus(t *testing.T) {
	status := NewServiceStatus("test-service")

	if status == nil {
		t.Fatal("NewServiceStatus returned nil")
	}

	if status.Name != "test-service" {
		t.Errorf("Expected name 'test-service', got %s", status.Name)
	}

	if status.GetStatus() != StatusStopped {
		t.Errorf("Expected initial status %s, got %s", StatusStopped, status.GetStatus())
	}
}

func TestServiceStatus_SetStatus(t *testing.T) {
	status := NewServiceStatus("test-service")

	status.SetStatus(StatusStarting)
	if status.GetStatus() != StatusStarting {
		t.Errorf("Expected status %s, got %s", StatusStarting, status.GetStatus())
	}

	status.SetStatus(StatusRunning)
	if status.GetStatus() != StatusRunning {
		t.Errorf("Expected status %s, got %s", StatusRunning, status.GetStatus())
	}

	if status.StartedAt.IsZero() {
		t.Error("StartedAt should be set when status is Running")
	}

	if status.GetError() != nil {
		t.Error("Error should be cleared when status is Running")
	}
}

func TestServiceStatus_SetError(t *testing.T) {
	status := NewServiceStatus("test-service")

	err := errors.New("test error")
	status.SetError(err)

	if status.GetStatus() != StatusError {
		t.Errorf("Expected status %s, got %s", StatusError, status.GetStatus())
	}

	if status.GetError() == nil {
		t.Error("Error should be set")
	}

	if status.GetError().Error() != "test error" {
		t.Errorf("Expected error 'test error', got %v", status.GetError())
	}
}

func TestServiceStatus_IsRunning(t *testing.T) {
	status := NewServiceStatus("test-service")

	if status.IsRunning() {
		t.Error("Service should not be running initially")
	}

	status.SetStatus(StatusRunning)
	if !status.IsRunning() {
		t.Error("Service should be running")
	}

	status.SetStatus(StatusStopped)
	if status.IsRunning() {
		t.Error("Service should not be running when stopped")
	}
}

func TestServiceStatus_GetUptime(t *testing.T) {
	status := NewServiceStatus("test-service")

	uptime := status.GetUptime()
	if uptime != 0 {
		t.Errorf("Expected uptime 0 for stopped service, got %v", uptime)
	}

	status.SetStatus(StatusRunning)
	time.Sleep(100 * time.Millisecond)

	uptime = status.GetUptime()
	if uptime == 0 {
		t.Error("Uptime should be greater than 0 for running service")
	}

	if uptime < 100*time.Millisecond || uptime > 200*time.Millisecond {
		t.Errorf("Uptime should be around 100ms, got %v", uptime)
	}

	status.SetStatus(StatusStopped)
	uptime = status.GetUptime()
	if uptime != 0 {
		t.Errorf("Expected uptime 0 for stopped service, got %v", uptime)
	}
}

func TestServiceStatus_ConcurrentAccess(t *testing.T) {
	status := NewServiceStatus("test-service")

	done := make(chan bool)
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				status.SetStatus(StatusRunning)
				status.GetStatus()
				status.IsRunning()
				status.GetUptime()
				status.SetStatus(StatusStopped)
			}
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	if status.GetStatus() == StatusError {
		t.Error("Status should not be in error state after concurrent access")
	}
}

