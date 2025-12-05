package main

import (
	"fmt"
	"os"
	
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/shared/storage"
	"github.com/vzahanych/view-guard-meta/user-vm-api/internal/model-catalog"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test-validation <models_dir> [model_id]")
		fmt.Println("Example: test-validation /app/data/models baseline-yolov8n")
		os.Exit(1)
	}
	
	baseDir := os.Args[1]
	modelID := "baseline-yolov8n"
	if len(os.Args) >= 3 {
		modelID = os.Args[2]
	}
	
	modelStorage, err := storage.NewModelStorage(baseDir)
	if err != nil {
		fmt.Printf("❌ Failed to create ModelStorage: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Println("MODEL VALIDATION SERVICE TEST")
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Printf("Model Directory: %s\n", baseDir)
	fmt.Printf("Model ID: %s\n\n", modelID)
	
	// Check if model exists
	if !modelStorage.ModelExists(modelID) {
		fmt.Printf("❌ Model '%s' does not exist\n", modelID)
		os.Exit(1)
	}
	
	fmt.Println("=== Testing ModelStorage Validation ===")
	
	// Test ValidateModel (comprehensive)
	if err := modelStorage.ValidateModel(modelID); err != nil {
		fmt.Printf("❌ ValidateModel failed: %v\n", err)
	} else {
		fmt.Printf("✅ ValidateModel passed\n")
	}
	
	// Test ValidateModelSize
	if err := modelStorage.ValidateModelSize(modelID); err != nil {
		fmt.Printf("❌ ValidateModelSize failed: %v\n", err)
	} else {
		fmt.Printf("✅ ValidateModelSize passed\n")
	}
	
	// Test ValidateModelFormat
	if err := modelStorage.ValidateModelFormat(modelID); err != nil {
		fmt.Printf("❌ ValidateModelFormat failed: %v\n", err)
	} else {
		fmt.Printf("✅ ValidateModelFormat passed\n")
	}
	
	fmt.Println("\n=== Testing ModelValidator ===")
	validator := modelcatalog.NewModelValidator(modelStorage)
	
	// Test ValidateModel (comprehensive validation)
	result, err := validator.ValidateModel(modelID)
	if err != nil {
		fmt.Printf("❌ ModelValidator.ValidateModel error: %v\n", err)
	} else {
		if result.Valid {
			fmt.Printf("✅ ModelValidator.ValidateModel: VALID\n")
		} else {
			fmt.Printf("❌ ModelValidator.ValidateModel: INVALID\n")
			for _, e := range result.Errors {
				fmt.Printf("  Error: %s\n", e)
			}
		}
		if len(result.Warnings) > 0 {
			for _, w := range result.Warnings {
				fmt.Printf("  Warning: %s\n", w)
			}
		}
	}
	
	// Test ValidateEdgeCompatibility
	result2, err := validator.ValidateEdgeCompatibility(modelID)
	if err != nil {
		fmt.Printf("❌ ValidateEdgeCompatibility error: %v\n", err)
	} else {
		if result2.Valid {
			fmt.Printf("✅ ValidateEdgeCompatibility: VALID\n")
		} else {
			fmt.Printf("❌ ValidateEdgeCompatibility: INVALID\n")
			for _, e := range result2.Errors {
				fmt.Printf("  Error: %s\n", e)
			}
		}
		if len(result2.Warnings) > 0 {
			for _, w := range result2.Warnings {
				fmt.Printf("  Warning: %s\n", w)
			}
		}
	}
	
	// Get model info for additional verification
	modelInfo, err := modelStorage.GetModelInfo(modelID)
	if err == nil {
		fmt.Println("\n=== Model Information ===")
		fmt.Printf("Model ID: %s\n", modelInfo.ModelID)
		fmt.Printf("Size: %d bytes (%.2f MB)\n", modelInfo.SizeBytes, float64(modelInfo.SizeBytes)/(1024*1024))
		fmt.Printf("Max Size: %d bytes (%.2f MB)\n", storage.MaxModelSizeBytes, float64(storage.MaxModelSizeBytes)/(1024*1024))
		if modelInfo.Metadata != nil {
			fmt.Printf("Model Type: %s\n", modelInfo.Metadata.ModelType)
			fmt.Printf("Framework: %s\n", modelInfo.Metadata.Framework)
			fmt.Printf("Input Shape: %v\n", modelInfo.Metadata.InputShape)
		}
	}
	
	fmt.Println("\n" + "=" + string(make([]byte, 60)))
	fmt.Println("✅ VALIDATION TEST COMPLETE")
	fmt.Println("=" + string(make([]byte, 60)))
}

