package finder

import (
	"os"
	"path/filepath"
)

func FindHelmChartDirs(root string) ([]string, error) {
	var chartDirs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			chartYamlPath := filepath.Join(path, "Chart.yaml")
			stat, err := os.Stat(chartYamlPath)
			if err == nil && stat.Mode().IsRegular() {
				chartDirs = append(chartDirs, path)
			}
		}
		return nil
	})
	return chartDirs, err
}
