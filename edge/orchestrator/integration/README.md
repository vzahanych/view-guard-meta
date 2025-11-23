# Integration Tests

Integration tests verify that multiple components work together correctly. Unlike unit tests which test individual components in isolation, integration tests verify the interaction between components.

## Test Structure

Integration tests are located in the `integration/` directory and follow the naming convention `*_integration_test.go`.

## Running Integration Tests

```bash
# Run all integration tests
go test ./integration/... -v

# Run specific integration test
go test ./integration/... -run TestServiceManagerIntegration -v

# Run with build tags (if needed)
go test -tags=integration ./integration/... -v
```

## Test Categories

### 1. Service Integration Tests
Tests that verify services work together:
- Service manager lifecycle
- Service communication via event bus
- Health check integration

### 2. Configuration & State Integration Tests
Tests that verify configuration and state management:
- Configuration loading and state persistence
- State recovery on restart
- Configuration changes affecting state

### 3. Camera & Video Integration Tests
Tests that verify camera and video processing:
- Camera discovery → registration → video processing
- Frame extraction → storage
- Clip recording → storage → retention

### 4. Storage Integration Tests
Tests that verify storage management:
- Clip storage → retention policy
- Snapshot generation → storage
- Disk monitoring → retention enforcement

## Test Environment

Integration tests use:
- Temporary directories for data storage
- In-memory or temporary databases
- Mock external dependencies where appropriate
- Real component instances (not mocks)

## Best Practices

1. **Isolation**: Each test should be independent and not rely on other tests
2. **Cleanup**: Always clean up resources (files, databases) after tests
3. **Real Components**: Use real component instances, not mocks, to catch integration issues
4. **Error Scenarios**: Test both success and failure paths
5. **Performance**: Integration tests may be slower than unit tests, but should still complete in reasonable time

