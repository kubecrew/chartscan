package finder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindHelmChartDirs(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	chartDir := filepath.Join(tempDir, "chart")
	os.Mkdir(chartDir, 0755)
	os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("apiVersion: v2"), 0644)

	chartDirs, err := FindHelmChartDirs(tempDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(chartDirs) != 1 || chartDirs[0] != chartDir {
		t.Fatalf("Expected [%s], got %v", chartDir, chartDirs)
	}
}

func TestFindHelmChartDirs_EmptyDir(t *testing.T) {
	tempDir := t.TempDir()

	chartDirs, err := FindHelmChartDirs(tempDir)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(chartDirs) != 0 {
		t.Fatalf("Expected 0 chart dirs, got %d", len(chartDirs))
	}
}

func TestFindHelmChartDirs_NonExistentDir(t *testing.T) {
	_, err := FindHelmChartDirs("/non/existent/path/123456789")
	if err == nil {
		t.Fatalf("Expected error for non-existent directory, got nil")
	}
}
