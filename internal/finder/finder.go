package finder

import (
	"os"
	"path/filepath"
)

// FindHelmChartDirs finds all directories in the file tree rooted at root that contain a Chart.yaml file.
// It returns a slice of strings that stores the paths to the Helm chart directories and an error if an error occurs while walking the tree.
// If the root is empty, it returns an empty slice and a nil error.
func FindHelmChartDirs(root string) ([]string, error) {
	// chartDirs is a slice of strings that stores the paths to the Helm chart directories.
	var chartDirs []string
	// filepath.Walk walks the file tree rooted at root, calling walkFn for each file or directory
	// in the tree, including root. All errors that occur while walking the tree are reported.
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		// If an error occurs while walking the tree, return it.
		if walkErr != nil {
			return walkErr
		}
		// If the current path is a directory, check if it contains a Chart.yaml file.
		if info.IsDir() {
			// ChartYamlPath is the path to the Chart.yaml file.
			chartYamlPath := filepath.Join(path, "Chart.yaml")
			// stat is the result of calling os.Stat on the Chart.yaml file.
			stat, err := os.Stat(chartYamlPath)
			// If the file exists and is a regular file, append the path to the chartDirs slice.
			if err == nil && stat.Mode().IsRegular() {
				chartDirs = append(chartDirs, path)
			}
		}
		// Return nil to indicate that no error occurred.
		return nil
	})
	// Return the chartDirs slice and the error from the filepath.Walk call.
	return chartDirs, err
}
