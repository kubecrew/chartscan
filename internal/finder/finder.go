package finder

import (
	"os"
	"path/filepath"
)

func FindHelmChartDirs(root string) ([]string, error) {
	var chartDirs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			chartPath := filepath.Join(path, "Chart.yaml")
			if _, err := os.Stat(chartPath); err == nil {
				chartDirs = append(chartDirs, path)
			}
		}
		return nil
	})
	return chartDirs, err
}
