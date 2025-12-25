.PHONY: generate-mocks test clean-mocks

# Mock generation using go.uber.org/mock (formerly gomock)
# Mocks are generated using //go:generate directives in each interface file
# Each service's mock is placed in its own mocks/ directory

# Generate all mocks using go generate (reads //go:generate directives from source files)
generate-mocks:
	@echo "Generating mocks for service interfaces..."
	@go generate ./...
	@echo "Mocks generated successfully!"

# Run tests
test:
	go test ./... -v

# Clean generated mocks from all service directories
clean-mocks:
	@echo "Cleaning generated mocks..."
	@find internal -type d -name mocks -exec rm -rf {} + 2>/dev/null || true
	@echo "Mocks cleaned successfully!"

