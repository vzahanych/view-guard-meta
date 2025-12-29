# AI Gateway Refactoring Plan

**Date**: 2025-12-28  
**Target Documents**: 
- `edge/orchestrator/internal/state-mng/WORKFLOW_AND_BUSINESS_LOGIC.md`
- `edge/orchestrator/internal/state-mng/REFACTORING_PLAN.md`
- `edge/orchestrator/internal/ai-gateway/AI_GATEWAY_DEVICE_AGNOSTIC_IMPLEMENTATION_PLAN.md` (existing plan)
- `edge/orchestrator/internal/vm-gateway/doc.go` (architectural pattern reference)

**Scope**: Complete refactoring of `ai-gateway` package to align with production workflow requirements and follow vm-gateway architectural pattern  
**Backward Compatibility**: Not required

---

## Executive Summary

This refactoring plan brings the AI Gateway service implementation into full compliance with the production workflow specification and aligns it with the vm-gateway architectural pattern. The current implementation is camera-centric, lacks production features (circuit breaker, health monitoring, proper resource limits), and doesn't follow the provider-agnostic architecture pattern. This plan incorporates the existing device-agnostic implementation plan while adding comprehensive production features.

**Key Transformation Areas**:
1. **Provider-agnostic architecture**: Follow vm-gateway pattern with interface, types, and implementation separation
2. **Device-agnostic design**: Transform from camera-centric to device-agnostic (Option A: IoT DataProcessor, Option B: task-based service)
3. **Task-based execution**: State manager creates inference tasks; AI gateway executes them in parallel with resource limits
4. **Production features**: Circuit breaker, health monitoring, observability, proper timeouts and retries
5. **Model-aware inference**: Per-device model selection (no global default model)
6. **Resource management**: Bounded parallelism, capacity limits, no queuing (reject if at capacity)

---

## Epic 1: Provider-Agnostic Architecture (Following vm-gateway Pattern)

**Goal**: Restructure the codebase to follow the vm-gateway architectural pattern with clear separation of concerns.

### Section 1.1: Interface and Types Separation

#### Subsection 1.1.1: Main Interface File
- **Files**: `ai_gateway.go` (rename from `ai-gateway-iface.go`)
- **Changes**:
  - Define `AIGateway` interface (main service interface)
  - Define sentinel errors (similar to vm-gateway):
    - `ErrNotInitialized`
    - `ErrAlreadyStarted`
    - `ErrAtCapacity` (when service is at capacity and cannot accept tasks)
    - `ErrInvalidDevice` (invalid device or device data)
    - `ErrInferenceFailed` (inference request failed)
    - `ErrModelNotFound` (model not found for device)
    - `ErrCircuitBreakerOpen` (circuit breaker is open)
  - Define factory function `NewAIGateway(ctx, config, logger, ...)`
  - Define provider function `AIGatewayProvider(lc, cfg, logger, ...)` with fx lifecycle
  - Add comprehensive package documentation (similar to vm-gateway/doc.go)
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 1.1.2: Types Package Structure
- **Files**: `types/` directory (already exists, enhance)
- **Changes**:
  - Move all configuration types to `types/config.go`
  - Create `types/task.go` for task-based types:
    - `InferenceTask` struct (TaskID, DeviceID, DeviceType, ModelMetadata, DeviceData, Context)
    - `TaskStatus` enum (pending, executing, completed, failed, rejected)
  - Create `types/health.go` for health-related types:
    - `HealthStatus` enum (healthy, degraded, circuit_breaker_open, unhealthy)
    - `AIGatewayHealth` struct (status, metrics, circuit breaker state, capacity)
  - Create `types/circuit_breaker.go` for circuit breaker types:
    - `CircuitBreakerState` enum (closed, open, half_open)
    - `CircuitBreakerConfig` struct (threshold, timeout, etc.)
  - Create `types/provider.go` for provider interface:
    - `AIServiceProvider` interface (provider-agnostic AI service operations)
    - Provider-specific configuration types
  - Create `types/errors.go` for error types
- **Dependencies**: 1.1.1
- **Estimated Effort**: 1 day

#### Subsection 1.1.3: Implementation Package Structure
- **Files**: `impl/` directory (already exists, enhance)
- **Changes**:
  - Create `impl/ai_gateway_impl.go` (main implementation, rename from `ai-gateway-impl.go`)
  - Create provider-specific implementations:
    - `impl/aiservice/aiservice_provider.go` (AI service HTTP client implementation)
    - `impl/mock/mock_provider.go` (future: mock provider for testing)
  - Each provider implements `types.AIServiceProvider` interface
  - Main implementation delegates to provider
- **Dependencies**: 1.1.2
- **Estimated Effort**: 2 days

### Section 1.2: Lifecycle Management

#### Subsection 1.2.1: Service Lifecycle
- **Files**: `impl/ai_gateway_impl.go`
- **Changes**:
  - Implement `Start(ctx)` method:
    - Initialize provider (AI service client)
    - Verify connectivity (health check)
    - Initialize worker pool (if task-based architecture)
    - Initialize circuit breaker
    - Start background tasks (health monitoring, circuit breaker recovery)
  - Implement `Stop(ctx)` method:
    - Stop background tasks gracefully
    - Stop worker pool (wait for in-flight tasks, max timeout)
    - Close provider connections
    - Flush pending operations
  - Follow vm-gateway pattern: service owns lifecycle of sub-components
  - **Critical**: No stored contexts in constructor (contexts flow from callers)
- **Dependencies**: 1.1.3
- **Estimated Effort**: 1 day

#### Subsection 1.2.2: Provider Lifecycle
- **Files**: `impl/aiservice/aiservice_provider.go`
- **Changes**:
  - Implement provider-specific initialization
  - Implement provider-specific cleanup
  - Providers do NOT register their own fx.Lifecycle hooks (gateway-owned lifecycle pattern)
- **Dependencies**: 1.2.1
- **Estimated Effort**: 1 day

---

## Epic 2: Device-Agnostic Architecture

**Goal**: Transform the codebase from camera-centric to device-agnostic terminology and types.

### Section 2.1: Type System Refactoring

#### Subsection 2.1.1: Replace CameraID with DeviceID
- **Files**: All files in `ai_gateway.go`, `types/`, `impl/`
- **Changes**:
  - Replace all `CameraID` references with `DeviceID`
  - Update function signatures:
    - `StartFrameProcessing(ctx, cameraID)` → `ExecuteTask(ctx, task *InferenceTask)` (task-based)
    - `StopFrameProcessing(ctx, cameraID)` → `CancelTask(ctx, taskID string)` (task-based)
    - `GetProcessingStats(cameraID)` → `GetProcessingStats(deviceID string)`
  - Update type definitions: `ProcessingStats.CameraID` → `DeviceID`
  - Update variable names and map keys
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 2.1.2: Device-Agnostic Type Definitions
- **Files**: `types/types.go`
- **Changes**:
  - Create `DeviceID` type alias (string)
  - Create `DeviceType` enum (camera, sensor, audio_device, etc.)
  - Update `ModelMetadata` struct:
    - `CameraID *string` → `DeviceID string` (required, not optional)
    - Add `DeviceType string` field
  - Update `ProcessingStats` struct:
    - `CameraID` → `DeviceID`
    - Add `DeviceType` field
  - Update `InferenceRequest` to be device-agnostic:
    - Remove camera-specific fields
    - Add `ModelID` and `ModelPath` (for model selection)
  - Update `DetectionResult` struct:
    - `CameraID` → `DeviceID`
    - Add `DeviceType` field
- **Dependencies**: 2.1.1
- **Estimated Effort**: 1 day

#### Subsection 2.1.3: Device Data Integration
- **Files**: `types/task.go`
- **Changes**:
  - Define `InferenceTask` struct:
    ```go
    type InferenceTask struct {
        TaskID        string
        DeviceID      string
        DeviceType    string
        ModelMetadata *ModelMetadata
        DeviceData    *iottypes.DeviceData // Raw device data (video frame, sensor reading, etc.)
        Context       context.Context
        Timestamp     time.Time
    }
    ```
  - Define `TaskResult` struct:
    ```go
    type TaskResult struct {
        TaskID        string
        Success       bool
        Response      *InferenceResponse
        SecurityEvent *statemng.SecurityEvent // If detection found
        Error         error
        Duration      time.Duration
    }
    ```
- **Dependencies**: 2.1.2
- **Estimated Effort**: 1 day

---

## Epic 3: Task-Based Architecture (Incorporating Existing Plan)

**Goal**: Implement task-based architecture where state manager creates tasks and AI gateway executes them.

### Section 3.1: Task-Based Service Interface

#### Subsection 3.1.1: Service Interface Redesign
- **Files**: `ai_gateway.go`
- **Changes**:
  - Redesign `AIGateway` interface for task-based execution:
    ```go
    type AIGateway interface {
        common.Service // Start, Stop, Name
        
        // ExecuteTask executes an inference task (non-blocking, parallel execution)
        // Returns error if task cannot be accepted (at capacity, invalid task, circuit breaker open)
        // Task execution happens asynchronously in worker pool
        ExecuteTask(ctx context.Context, task *types.InferenceTask) error
        
        // CancelTask cancels a running task
        CancelTask(ctx context.Context, taskID string) error
        
        // SetEventCallback sets callback for security events
        SetEventCallback(callback func(*statemng.SecurityEvent))
        
        // SetConfidenceThreshold updates confidence threshold
        SetConfidenceThreshold(threshold float64)
        
        // GetConfidenceThreshold returns current threshold
        GetConfidenceThreshold() float64
        
        // GetProcessingStats returns stats for a device
        GetProcessingStats(deviceID string) (*types.ProcessingStats, error)
        
        // HealthSnapshot returns service health and status
        HealthSnapshot() types.AIGatewayHealth
        
        // GetCapacity returns current capacity (available slots for parallel tasks)
        GetCapacity() int // Returns: MaxParallelTasks - ActiveTasks
    }
    ```
  - Remove camera-specific methods: `StartFrameProcessing`, `StopFrameProcessing`, `ProcessFrame`
  - Keep `NotifyModelDeployment` (updated for device-agnostic)
  - **Task creation responsibility**: 
    - State manager creates `InferenceTask` objects and calls `ExecuteTask`
    - AI gateway does NOT create tasks itself; it only executes tasks provided by state manager
    - State manager is responsible for task lifecycle (creation, scheduling, cancellation)
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 3.1.2: Task Executor Implementation
- **Files**: `impl/task_executor.go` (new file)
- **Changes**:
  - Implement `TaskExecutor` interface:
    ```go
    type TaskExecutor interface {
        Execute(ctx context.Context, task *types.InferenceTask) error
        Cancel(ctx context.Context, taskID string) error
        Start(ctx context.Context) error
        Stop(ctx context.Context) error
        GetStatus() ExecutorStatus
    }
    ```
  - Implement worker pool-based executor:
    - Buffered channel with size = `MaxParallelTasks` (acts as semaphore)
    - No queuing: channel send blocks if full → task rejected immediately
    - Worker pool: fixed number of workers process tasks in parallel
    - Resource limits: channel capacity limits concurrent execution
  - Implement task execution:
    - **Before executing task**: verify device is connected (query IoT service for device state)
      - If device is disconnected: reject task with `ErrInvalidDevice`
      - If device state is unavailable: reject task with `ErrInvalidDevice`
    - Load model metadata from task
    - Validate model metadata exists and is valid (return `ErrModelNotFound` if invalid)
    - Call AI service provider with device data and model
    - Process inference response
    - Emit security events if detections found
    - Update statistics
- **Dependencies**: 3.1.1, Section 1.1
- **Estimated Effort**: 3 days

---

## Epic 4: Circuit Breaker Implementation

**Goal**: Implement circuit breaker pattern for AI service failures as specified in workflow document.

### Section 4.1: Circuit Breaker Types

#### Subsection 4.1.1: Circuit Breaker Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `CircuitBreakerConfig` struct:
    - `FailureThreshold int` (default: 5 consecutive failures)
    - `OpenTimeout time.Duration` (default: 60s - how long circuit stays open)
    - `HalfOpenMaxRequests int` (default: 3 - max requests in half-open state)
    - `SuccessThreshold int` (default: 2 - successes needed to close from half-open)
  - Add circuit breaker configuration to `AIGatewayConfig`
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 4.1.2: Circuit Breaker Implementation
- **Files**: `impl/circuit_breaker.go` (new file)
- **Changes**:
  - Implement `CircuitBreaker` struct:
    - Track state (closed, open, half_open)
    - Track failure count
    - Track success count (in half-open state)
    - Track last failure time
  - Implement `Call(ctx, fn func() error) error`:
    - Check circuit state
    - If open: return `ErrCircuitBreakerOpen` (don't call function)
    - If half-open: allow limited requests, track success/failure
    - If closed: call function, track failures
    - Update state based on results
  - Implement state transitions:
    - Closed → Open (after failure threshold)
    - Open → HalfOpen (after timeout)
    - HalfOpen → Closed (after success threshold)
    - HalfOpen → Open (on failure)
  - Emit events: `ai.circuit_breaker_opened`, `ai.circuit_breaker_closed`
- **Dependencies**: 4.1.1, Section 1.1
- **Estimated Effort**: 2 days

#### Subsection 4.1.3: Circuit Breaker Integration
- **Files**: `impl/task_executor.go`
- **Changes**:
  - Integrate circuit breaker into task execution:
    - Wrap AI service calls with circuit breaker
    - If circuit breaker open: reject task immediately with `ErrCircuitBreakerOpen`
    - Track failures for circuit breaker
  - Update health status when circuit breaker opens/closes
- **Dependencies**: 4.1.2
- **Estimated Effort**: 1 day

---

## Epic 5: Production Features - Timeouts, Retries, and Resource Limits

**Goal**: Implement proper timeouts, retries, and resource limits as specified.

### Section 5.1: Timeout and Retry Configuration

#### Subsection 5.1.1: Timeout Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Update `AIGatewayConfig`:
    - `RequestTimeout time.Duration` (default: 10s per AI service call - single inference request)
    - `TaskTimeout time.Duration` (default: 30s per task, includes retries - aligns with state-mng frame processing interval)
    - `MaxRetries int` (default: 3)
    - `RetryDelay time.Duration` (default: 1s, exponential backoff)
    - `MaxParallelTasks int` (default: 10, resource limit)
    - `WorkerShutdownTimeout time.Duration` (default: 30s)
  - **Timeout clarification**:
    - `RequestTimeout`: Timeout for a single AI service HTTP call (10s default)
    - `TaskTimeout`: Total timeout for entire task execution including retries (30s default, aligns with state-mng's frame processing interval)
  - Implement validation and defaults
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 5.1.2: Retry Logic Implementation
- **Files**: `impl/retry_manager.go` (new file)
- **Changes**:
  - Implement `RetryManager` struct:
    - Track retry configuration
    - Calculate exponential backoff
  - Implement `Retry(ctx, fn func() error) error`:
    - Attempt function call
    - On failure: retry with exponential backoff
    - Respect max retries and timeout
    - Return last error if all retries fail
  - Integrate with task executor
- **Dependencies**: 5.1.1
- **Estimated Effort**: 1 day

#### Subsection 5.1.3: Resource Limits
- **Files**: `impl/task_executor.go`
- **Changes**:
  - Implement capacity management:
    - Track active tasks (atomic counter)
    - Track max parallel tasks (from config)
    - Reject tasks if at capacity (return `ErrAtCapacity`)
  - Implement `GetCapacity() int`:
    - Return available capacity (max - active)
  - Implement graceful shutdown:
    - Wait for in-flight tasks (max timeout)
    - Cancel remaining tasks after timeout
- **Dependencies**: 5.1.2
- **Estimated Effort**: 1 day

---

## Epic 6: Model-Aware Inference

**Goal**: Implement per-device model selection (no global default model).

### Section 6.1: Model Metadata Management

#### Subsection 6.1.1: Model Registry
- **Files**: `impl/model_registry.go` (new file)
- **Changes**:
  - Implement `ModelRegistry` struct:
    - Track deployed models per device: `map[DeviceID]*ModelMetadata`
    - Thread-safe access (mutex)
  - Implement `GetModelForDevice(ctx, deviceID) (*ModelMetadata, error)`:
    - Look up model for device
    - Return error if not found (no global default)
  - Implement `RegisterModel(ctx, deviceID, metadata) error`:
    - Register model for device (called on model deployment)
  - Implement `UnregisterModel(ctx, deviceID) error`:
    - Remove model for device (called on model removal)
- **Dependencies**: Epic 2
- **Estimated Effort**: 1 day

#### Subsection 6.1.2: Model Selection in Task Execution
- **Files**: `impl/task_executor.go`
- **Changes**:
  - Update task execution to use model from task:
    - Task includes `ModelMetadata` (provided by state manager)
    - Use model metadata for AI service call
    - No fallback to global default model
  - Validate model metadata before execution:
    - Check model exists and is valid
    - Return `ErrModelNotFound` if invalid
- **Dependencies**: 6.1.1
- **Estimated Effort**: 1 day

#### Subsection 6.1.3: Model Deployment Notification
- **Files**: `impl/ai_gateway_impl.go`
- **Changes**:
  - Update `NotifyModelDeployment` method:
    - Accept device-agnostic `ModelMetadata`
    - Register model in model registry
    - Notify AI service provider about model deployment
    - Handle device-agnostic metadata (DeviceID, DeviceType)
  - **CRITICAL: `NotifyModelDeployment` registers model metadata but DOES NOT activate inference**
    - Model verification is State Manager's responsibility (see state-mng Epic 5)
    - AI gateway must never activate unverified models
    - AI gateway only stores model metadata for use when State Manager requests inference
    - Inference activation is triggered by State Manager calling `ExecuteTask` with a task that includes the verified model
- **Dependencies**: 6.1.2
- **Estimated Effort**: 1 day

---

## Epic 7: IoT DataProcessor Integration (Option A - Preferred)

**Goal**: Implement AI gateway as IoT DataProcessor for device-agnostic integration.

### Section 7.1: DataProcessor Implementation

#### Subsection 7.1.1: DataProcessor Interface
- **Files**: `impl/data_processor.go` (new file)
- **Changes**:
  - Implement `iot.DataProcessor` interface:
    ```go
    type AIDataProcessor struct {
        gateway *aiGatewayImpl
        // ...
    }
    
    func (p *AIDataProcessor) Process(ctx context.Context, data *iottypes.DeviceData) error {
        // Get model for device
        // Create inference task
        // Execute task via gateway
    }
    
    func (p *AIDataProcessor) GetSupportedDataTypes() []iottypes.DeviceDataType {
        return []iottypes.DeviceDataType{
            iottypes.DeviceDataTypeVideoFrame,
            // Future: DeviceDataTypeAudioSample, etc.
        }
    }
    ```
  - Register processor with IoT service
  - State manager binds models to devices
- **Dependencies**: Epic 3, Epic 6
- **Estimated Effort**: 2 days

#### Subsection 7.1.2: Integration with IoT Service
- **Files**: `impl/ai_gateway_impl.go`
- **Changes**:
  - Remove direct CCTV service dependency
  - Add IoT service dependency (for processor registration)
  - Register data processor on startup
  - Unregister on shutdown
- **Dependencies**: 7.1.1
- **Estimated Effort**: 1 day

---

## Epic 8: Health Monitoring and Observability

**Goal**: Add comprehensive health monitoring following vm-gateway pattern.

### Section 8.1: Health Status Tracking

#### Subsection 8.1.1: Health Status Types
- **Files**: `types/health.go`
- **Changes**:
  - Define `HealthStatus` enum:
    - `HealthStatusHealthy`
    - `HealthStatusDegraded` (circuit breaker open, high failure rate)
    - `HealthStatusCircuitBreakerOpen` (circuit breaker is open)
    - `HealthStatusUnhealthy` (AI service unreachable, critical failures)
  - Define `AIGatewayHealth` struct:
    - `Status HealthStatus`
    - `CircuitBreakerState CircuitBreakerState`
    - `ActiveTasks int`
    - `MaxParallelTasks int`
    - `Capacity int` (available slots)
    - `TotalTasksProcessed int64`
    - `TotalTasksRejected int64`
    - `TotalEventsDetected int64`
    - `FailureRate float64` (recent failure rate)
    - `AIServiceHealth string` (provider-specific health)
    - `PerDeviceStats map[string]*ProcessingStats`
    - `LastUpdated time.Time`
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 8.1.2: Health Snapshot API
- **Files**: `ai_gateway.go`, `impl/ai_gateway_impl.go`
- **Changes**:
  - Add `HealthSnapshot() AIGatewayHealth` method to interface
  - Implement health snapshot:
    - Query circuit breaker state
    - Query task executor status
    - Query AI service provider health
    - Aggregate statistics
    - Calculate failure rates
    - Aggregate into `AIGatewayHealth` struct
  - Follow vm-gateway pattern for health snapshots
- **Dependencies**: 8.1.1, Epic 4, Epic 5
- **Estimated Effort**: 2 days

### Section 8.2: Operational Metrics

#### Subsection 8.2.1: Metrics Tracking
- **Files**: `impl/metrics.go` (new file)
- **Changes**:
  - Track operational metrics:
    - Tasks processed per second (by device)
    - Tasks rejected per second (by reason)
    - Events detected per second
    - Inference latency (P50, P95, P99)
    - Failure rates over time
    - Circuit breaker state transitions
    - Capacity utilization
  - Expose metrics via health snapshot or separate metrics endpoint
- **Dependencies**: 8.1.2
- **Estimated Effort**: 2 days

#### Subsection 8.2.2: Event Emission
- **Files**: `impl/ai_gateway_impl.go`
- **Changes**:
  - Add event bus dependency (similar to vm-gateway)
  - Emit operational events:
    - `ai.inference_started`, `ai.inference_completed`, `ai.inference_failed`
    - `ai.detection` (security event detected)
    - `ai.circuit_breaker_opened`, `ai.circuit_breaker_closed`
    - `ai.health_degraded`, `ai.health_recovered`
  - Use structured event types (similar to vm-gateway event types)
- **Dependencies**: 8.2.1
- **Estimated Effort**: 1 day

---

## Epic 9: Security and Response Validation

**Goal**: Implement security requirements (no raw frame logging, strict response validation).

### Section 9.1: Security Requirements

#### Subsection 9.1.1: Safe Logging
- **Files**: `impl/task_executor.go`, `impl/aiservice/aiservice_provider.go`
- **Changes**:
  - Ensure no raw frames in logs:
    - Log frame metadata (size, format, timestamp) but not content
    - Log device ID, model ID, but not frame data
    - Use structured logging with safe fields only
  - Audit logging for security-sensitive operations
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 9.1.2: Response Validation
- **Files**: `impl/aiservice/aiservice_provider.go`
- **Changes**:
  - Implement strict response validation:
    - Validate inference response structure
    - Validate bounding boxes (coordinates, confidence ranges)
    - Validate detection counts
    - Reject malformed responses
  - Implement bounded resource usage:
    - Limit response size
    - Limit number of detections per response
    - Timeout on slow responses
- **Dependencies**: 9.1.1
- **Estimated Effort**: 2 days

---

## Epic 10: Provider Implementation Refactoring

**Goal**: Refactor AI service provider to follow provider-agnostic pattern.

### Section 10.1: AI Service Provider Refactoring

#### Subsection 10.1.1: Provider Interface Implementation
- **Files**: `impl/aiservice/aiservice_provider.go` (rename from `ai_client.go`)
- **Changes**:
  - Implement `AIServiceProvider` interface:
    - `Infer(ctx, deviceData *iottypes.DeviceData, modelMetadata *ModelMetadata) (*InferenceResponse, error)`
    - `InferBatch(ctx, tasks []*InferenceTask) (*BatchInferenceResponse, error)`
    - `NotifyModelDeployment(ctx, metadata *ModelMetadata) error`
    - `HealthCheck(ctx) error`
    - `GetStats(ctx) (*InferenceStats, error)`
  - Remove camera-specific logic
  - Make provider-agnostic
  - Update to use `DeviceData` instead of `cctvtypes.Frame`
- **Dependencies**: Section 1.1, Epic 2
- **Estimated Effort**: 3 days

#### Subsection 10.1.2: Provider Configuration
- **Files**: `types/config.go`
- **Changes**:
  - Add `AIServiceProviderConfig` struct:
    - `ServiceURL string`
    - `Timeout time.Duration`
    - `MaxRetries int`
    - `RetryDelay time.Duration`
    - Provider-specific settings
  - Add provider-specific configuration to `AIGatewayConfig`
- **Dependencies**: 10.1.1
- **Estimated Effort**: 1 day

---

## Epic 11: Confidence and Class Filtering

**Goal**: Implement confidence threshold and class filtering as specified.

### Section 11.1: Filtering Implementation

#### Subsection 11.1.1: Confidence Threshold
- **Files**: `impl/filter_manager.go` (new file)
- **Changes**:
  - Implement `FilterManager` struct:
    - Track confidence threshold (per-device or global)
    - Track enabled classes (per-device or global)
  - Implement `FilterDetections(response *InferenceResponse, threshold float64, enabledClasses []string) *InferenceResponse`:
    - Filter bounding boxes by confidence threshold
    - Filter by enabled classes (if specified)
    - Return filtered response
  - Integrate with task execution
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 11.1.2: Per-Device Configuration
- **Files**: `impl/filter_manager.go`
- **Changes**:
  - Support per-device confidence thresholds:
    - Store thresholds per device
    - Use device-specific threshold if available, else global
  - Support per-device class filters:
    - Store enabled classes per device
    - Use device-specific classes if available, else global
- **Dependencies**: 11.1.1
- **Estimated Effort**: 1 day

---

## Epic 12: Degraded Mode and Failure Handling

**Goal**: Implement degraded mode for repeated failures with operator visibility.

### Section 12.1: Degraded Mode Detection

#### Subsection 12.1.1: Failure Tracking
- **Files**: `impl/failure_tracker.go` (new file)
- **Changes**:
  - Implement `FailureTracker` struct:
    - Track failure count per device
    - Track failure rate over time window
    - Track consecutive failures
  - Implement `RecordFailure(deviceID string)`
  - Implement `RecordSuccess(deviceID string)`
  - Implement `ShouldEnterDegradedMode(deviceID string) bool`:
    - Return true if failure rate > threshold (e.g., >50% over 1 hour)
    - Return true if consecutive failures > threshold
- **Dependencies**: None
- **Estimated Effort**: 1 day

#### Subsection 12.1.2: Degraded Mode Management
- **Files**: `impl/ai_gateway_impl.go`
- **Changes**:
  - Implement degraded mode:
    - Track degraded devices
    - Stop processing for degraded devices
    - Emit `ai.health_degraded` event with device details
    - Log to audit-log
  - Implement recovery:
    - Monitor degraded devices
    - Exit degraded mode when failure rate improves
    - Emit `ai.health_recovered` event
- **Dependencies**: 12.1.1, Section 1.1
- **Estimated Effort**: 2 days

---

## Epic 13: Documentation and Testing

**Goal**: Add comprehensive documentation and testing following vm-gateway pattern.

### Section 13.1: Documentation

#### Subsection 13.1.1: Package Documentation
- **Files**: `doc.go` (new file)
- **Changes**:
  - Add comprehensive package documentation (similar to vm-gateway/doc.go):
    - Architecture overview
    - Provider-agnostic design
    - Task-based execution model
    - Device-agnostic design
    - Circuit breaker pattern
    - Resource limits and capacity management
    - Configuration examples
    - Usage examples
    - Lifecycle management
    - Health monitoring
  - Document integration with IoT service (DataProcessor option)
  - Document integration with state manager (task creation)
- **Dependencies**: All epics
- **Estimated Effort**: 1 day

#### Subsection 13.1.2: API Documentation
- **Files**: All interface files
- **Changes**:
  - Add comprehensive method documentation
  - Document error conditions
  - Document return values
  - Add usage examples
  - Document task-based execution
  - Document circuit breaker behavior
- **Dependencies**: 13.1.1
- **Estimated Effort**: 1 day

### Section 13.2: Testing

#### Subsection 13.2.1: Unit Tests
- **Files**: `*_test.go` files
- **Changes**:
  - Test task execution
  - Test circuit breaker (state transitions, failure handling)
  - Test resource limits and capacity management
  - Test model selection per device
  - Test confidence and class filtering
  - Test degraded mode
  - Test health monitoring
  - Test provider abstraction
  - Test device-agnostic types
- **Dependencies**: All epics
- **Estimated Effort**: 3 days

#### Subsection 13.2.2: Integration Tests
- **Files**: `*_integration_test.go` files
- **Changes**:
  - Test full inference lifecycle (task creation → execution → event emission)
  - Test IoT DataProcessor integration
  - Test circuit breaker with real AI service failures
  - Test resource limits with concurrent tasks
  - Test degraded mode with repeated failures
  - Test health monitoring
- **Dependencies**: 13.2.1
- **Estimated Effort**: 2 days

---

## Implementation Order and Dependencies

### Phase 1: Foundation (Epics 1, 2)
- **Duration**: ~1.5 weeks
- **Epics**: 1 (Provider-Agnostic Architecture), 2 (Device-Agnostic Architecture)
- **Rationale**: Establishes the architectural foundation and type system

### Phase 2: Core Features (Epics 3, 4, 5)
- **Duration**: ~2.5 weeks
- **Epics**: 3 (Task-Based Architecture), 4 (Circuit Breaker), 5 (Timeouts, Retries, Resource Limits)
- **Rationale**: Implements core production features

### Phase 3: Model and Integration (Epics 6, 7)
- **Duration**: ~1.5 weeks
- **Epics**: 6 (Model-Aware Inference), 7 (IoT DataProcessor Integration)
- **Rationale**: Implements model management and IoT integration

### Phase 4: Production Features (Epics 8, 9, 10, 11, 12)
- **Duration**: ~2 weeks
- **Epics**: 8 (Health Monitoring), 9 (Security), 10 (Provider Refactoring), 11 (Filtering), 12 (Degraded Mode)
- **Rationale**: Completes production features

### Phase 5: Polish (Epic 13)
- **Duration**: ~1 week
- **Epics**: 13 (Documentation and Testing)
- **Rationale**: Completes documentation and testing

**Total Estimated Duration**: ~8.5 weeks

---

## Migration Notes

### Breaking Changes
- All `CameraID` references become `DeviceID`
- Camera-specific methods replaced with task-based methods
- Direct CCTV service dependency removed (replaced with IoT service)
- Configuration structure changes (circuit breaker, resource limits, timeouts)
- Model metadata structure changes (DeviceID instead of CameraID)

### Data Migration
- Existing processing stats may need migration (CameraID → DeviceID)
- Model metadata in storage needs migration (CameraID → DeviceID)

### Rollout Strategy
- Deploy to staging environment first
- Run full test suite (unit, integration)
- Verify task-based execution
- Verify circuit breaker behavior
- Verify resource limits
- Gradual rollout to production with monitoring
- Monitor inference latency and failure rates
- Rollback plan: revert to previous version if critical issues detected

---

## Success Criteria

1. ✅ Provider-agnostic architecture implemented (following vm-gateway pattern)
2. ✅ Device-agnostic types and methods implemented
3. ✅ Task-based architecture implemented and tested
4. ✅ Circuit breaker implemented and tested
5. ✅ Resource limits and capacity management implemented
6. ✅ Model-aware inference (per-device model selection) implemented
7. ✅ IoT DataProcessor integration implemented (Option A)
8. ✅ Health monitoring implemented and tested
9. ✅ Degraded mode implemented and tested
10. ✅ Security requirements implemented (no raw frame logging, response validation)
11. ✅ Comprehensive documentation added
12. ✅ Full test coverage (unit, integration)
13. ✅ Health snapshot API implemented
14. ✅ Event emission implemented

---

## Notes

- **No backward compatibility required**: This is a complete refactoring
- **No source code changes in this plan**: This document only defines the plan
- **Implementation should follow the exact specifications in WORKFLOW_AND_BUSINESS_LOGIC.md**
- **Architecture should follow vm-gateway pattern** (but simpler, as ai-gateway is a simpler service)
- **Device-agnostic implementation is mandatory** (Option A: IoT DataProcessor preferred, Option B: task-based service)
- **Task-based architecture is preferred** (state manager creates tasks, AI gateway executes them)
- **Circuit breaker is critical** (must trigger degraded mode on repeated failures)
- **Resource limits are mandatory** (bounded parallelism, no queuing, reject if at capacity)

---

**Document Status**: Ready for implementation  
**Next Steps**: Review plan, assign epics to development teams, begin Phase 1 implementation

