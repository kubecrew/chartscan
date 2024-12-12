package renderer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRenderHelmChart tests rendering both valid and invalid Helm charts
func TestRenderHelmChart(t *testing.T) {
	// Ensure Helm is installed and available
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("Helm CLI is not installed, skipping test")
	}

	// Valid chart test case
	t.Run("Valid chart", func(t *testing.T) {
		// Create a temporary directory with a valid Chart.yaml
		tempDir := t.TempDir()
		validChart := []byte("apiVersion: v2\nname: test\nversion: 0.1.0")
		if err := os.WriteFile(filepath.Join(tempDir, "Chart.yaml"), validChart, 0644); err != nil {
			t.Fatalf("Failed to write Chart.yaml: %v", err)
		}

		// Test the valid chart rendering
		success, errMessage := RenderHelmChart(tempDir)
		if !success {
			t.Fatalf("Expected rendering to succeed for valid chart, but got error: %s", errMessage)
		}
	})

	// Invalid chart test case
	t.Run("Invalid chart", func(t *testing.T) {
		// Create a temporary directory with an invalid Chart.yaml (missing version)
		tempDir := t.TempDir()
		invalidChart := []byte("apiVersion: v2\nname: test")
		if err := os.WriteFile(filepath.Join(tempDir, "Chart.yaml"), invalidChart, 0644); err != nil {
			t.Fatalf("Failed to write Chart.yaml: %v", err)
		}

		// Test the invalid chart rendering
		success, errMessage := RenderHelmChart(tempDir)
		if success {
			t.Fatalf("Expected rendering to fail for invalid chart, but it succeeded")
		}

		// Ensure an error message was returned
		if errMessage == "" {
			t.Fatalf("Expected an error message for invalid chart, but got none")
		}
	})
}
