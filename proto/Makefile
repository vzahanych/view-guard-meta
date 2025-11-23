.PHONY: proto generate clean

# Protocol buffer compiler
PROTOC := protoc
PROTOC_GEN_GO := protoc-gen-go
PROTOC_GEN_GO_GRPC := protoc-gen-go-grpc

# Directories
PROTO_DIR := proto
GO_OUT_DIR := go/generated
PYTHON_OUT_DIR := python/generated
TYPESCRIPT_OUT_DIR := typescript/generated

# Proto files
EDGE_PROTO_FILES := $(wildcard $(PROTO_DIR)/edge/*.proto)
KVM_PROTO_FILES := $(wildcard $(PROTO_DIR)/kvm/*.proto)

proto: generate

generate: generate-go

generate-go:
	@echo "Generating Go stubs..."
	@mkdir -p $(GO_OUT_DIR)
	@for proto_file in $(EDGE_PROTO_FILES) $(KVM_PROTO_FILES); do \
		dir=$$(dirname $$proto_file); \
		package=$$(basename $$dir); \
		$(PROTOC) \
			--go_out=$(GO_OUT_DIR) \
			--go_opt=paths=source_relative \
			--go-grpc_out=$(GO_OUT_DIR) \
			--go-grpc_opt=paths=source_relative \
			--proto_path=$(PROTO_DIR) \
			$$proto_file || exit 1; \
	done
	@echo "Go stubs generated in $(GO_OUT_DIR)"

clean:
	@echo "Cleaning generated files..."
	@rm -rf $(GO_OUT_DIR)
	@rm -rf $(PYTHON_OUT_DIR)
	@rm -rf $(TYPESCRIPT_OUT_DIR)

