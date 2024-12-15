package renderer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"

	"github.com/Jaydee94/chartscan/internal/models"
)

// TemplateParser parses a template file and extracts value references.
// It returns an array of value references and an error.
func TemplateParser(templateFile string) ([]models.ValueReference, error) {
	// Read the template file
	templateBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}

	templateString := string(templateBytes)
	// Initialize an empty array to store value references
	var valueReferences []models.ValueReference

	// Regex to capture dot notation values like .Values.service.port
	re := regexp.MustCompile(`{{\s*\.Values\.([a-zA-Z0-9_.\[\]-]+)\s*}}`)
	// Split the template string into lines
	lines := strings.Split(templateString, "\n")
	// Iterate over each line
	for i, line := range lines {
		// Find all matches of the regex in the line
		matches := re.FindAllStringSubmatchIndex(line, -1)

		// Iterate over each match
		for _, match := range matches {
			// Extract the matched value reference
			reference := line[match[2]:match[3]]
			// Check if the reference is not empty
			if reference == "" {
				// Return an error if the reference is empty
				return nil, fmt.Errorf("empty value reference: %s", line[match[0]:match[1]])
			}

			// Create a value reference model
			valueReference := models.ValueReference{
				Name:     reference,
				File:     templateFile,
				Line:     i + 1,
				FullText: line[match[0]:match[1]],
			}
			// Append the value reference to the array
			valueReferences = append(valueReferences, valueReference)
		}
	}

	// Return the array of value references and no error
	return valueReferences, nil
}

// ValuesLoader loads values from a YAML file
//
// This function reads a YAML file and unmarshals it into a map[string]interface{}
// It returns the map and an error if the file does not exist or if the YAML is invalid
func ValuesLoader(valuesFile string) (map[string]interface{}, error) {
	// Read the YAML file
	valuesBytes, err := os.ReadFile(valuesFile)
	if err != nil {
		// Return an error if the file does not exist or if the read operation failed
		return nil, err
	}

	// Create a map to store the values
	var values map[string]interface{}
	// Unmarshal the YAML bytes into the map
	err = yaml.Unmarshal(valuesBytes, &values)
	if err != nil {
		// Return an error if the YAML is invalid
		return nil, err
	}

	// Return the map and no error
	return values, nil
}

// CheckValueReferences takes a slice of ValueReferences and a map of values and checks for any undefined values
// It returns a slice of strings containing any undefined values found
func CheckValueReferences(valueReferences []models.ValueReference, values map[string]interface{}) []string {
	// Initialize an empty array to store any undefined values
	undefinedValues := make([]string, 0, len(valueReferences))

	// Iterate over each value reference
	for _, valueReference := range valueReferences {
		// Split the value reference name into a slice of keys
		keys := strings.Split(valueReference.Name, ".")
		// Check if the nested value exists in the values map
		if !checkNestedValueExists(keys, values) {
			// If the value does not exist, add an error to the undefinedValues slice
			undefinedValues = append(undefinedValues, fmt.Sprintf("Undefined value: '%s' referenced in %s at line %d", valueReference.Name, valueReference.File, valueReference.Line))
		}
	}

	// Return the slice of undefined values
	return undefinedValues
}

// checkNestedValueExists takes a slice of keys and a map of values and checks if the nested value referred to by the keys exists in the values map
// It returns true if the value exists and false if it does not
func checkNestedValueExists(keys []string, currentMap interface{}) bool {
	// If there are no keys or the current map is nil, return false
	if len(keys) == 0 || currentMap == nil {
		return false
	}

	// Start checking only the nested structure after .Values (no need to check .Values itself)
	switch v := currentMap.(type) {
	case map[string]interface{}:
		// If it's the last key in the path, check if it exists
		if len(keys) == 1 {
			_, exists := v[keys[0]]
			return exists
		}

		// If there are more keys, continue recursively
		if nextMap, exists := v[keys[0]].(map[string]interface{}); exists {
			return checkNestedValueExists(keys[1:], nextMap)
		}

		return false // Key not found at this level

	default:
		return false // If the current value is not a map, return false
	}
}

// mergeMaps merges two maps, combining nested maps recursively.
// If a key in both maps contains a map as its value, the function will
// merge these maps. Otherwise, the value from the source map will overwrite
// the value in the target map.
//
// Parameters:
//
//	target - The map that will be modified to include values from the source map.
//	source - The map whose values will be merged into the target map.
func mergeMaps(target, source map[string]interface{}) {
	for key, value := range source {
		// Check if both target and source values are maps, and merge them recursively
		if targetMap, ok := target[key].(map[string]interface{}); ok {
			if sourceMap, ok := value.(map[string]interface{}); ok {
				mergeMaps(targetMap, sourceMap)
				continue
			}
		}
		// Otherwise, set the value from source into the target map
		target[key] = value
	}
}

// Refactored RenderHelmChart function and its components

// ScanHelmChart renders a Helm chart and checks for undefined values in the chart and values files.
// The function takes a chart path and a slice of values files as input and returns a boolean indicating
// success or failure, a slice of errors encountered, a map of values and a slice of undefined values.
func ScanHelmChart(chartPath string, valuesFiles []string) (bool, []string, map[string]interface{}, []string) {
	if chartPath == "" {
		return false, []string{"Chart path is empty"}, nil, nil
	}

	// Check and handle dependencies
	success, errors := handleDependencies(chartPath)
	if !success {
		return false, errors, nil, nil
	}

	// Check values files existence
	missingFilesErrors := checkValuesFilesExistence(valuesFiles)
	if len(missingFilesErrors) > 0 {
		return false, missingFilesErrors, nil, nil
	}

	// Lint the chart
	lintErrors := lintChart(chartPath, valuesFiles)

	// Parse templates and gather value references
	valueReferences, templateErrors := parseTemplates(chartPath)
	lintErrors = append(lintErrors, templateErrors...)

	// Load values and merge additional values files
	values, loadErrors := loadAndMergeValues(chartPath, valuesFiles)
	lintErrors = append(lintErrors, loadErrors...)

	// Check for undefined values
	undefinedValues := CheckValueReferences(valueReferences, values)

	// Combine all errors
	allErrors := append(lintErrors, undefinedValues...)

	// Determine success
	success = len(allErrors) == 0

	// Defer cleanup of dependencies after linting and value checks
	defer cleanupDependencies(chartPath)

	return success, allErrors, values, undefinedValues
}

// TemplateHelmChart renders a Helm chart using the `helm template` command and outputs the results to stdout or a file.
// It takes a chart path, release name, output file (optional), and additional values files (optional).
// It returns any error encountered during the process.
func TemplateHelmChart(chartPath string, valuesFiles []string, outputFile string) error {
	// Ensure the chartPath is not empty
	if chartPath == "" {
		return fmt.Errorf("chart path is empty")
	}

	// Determine the release name (the folder name of the chart path)
	_, releaseName := filepath.Split(chartPath)

	// If the release name is ".", we should use the last part of the current directory as the release name
	if releaseName == "." {
		// Get the current working directory
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("error getting current directory: %v", err)
		}

		// Get the last part of the current directory as the release name
		_, releaseName = filepath.Split(currentDir)
	}

	releaseName = strings.TrimSpace(releaseName)

	// Validate that the release name is valid according to Helm's regex
	if !isValidReleaseName(releaseName) {
		return fmt.Errorf("invalid release name: %s", releaseName)
	}

	// Check and handle dependencies
	success, errors := handleDependencies(chartPath)
	if !success {
		return fmt.Errorf("error building dependencies: %s", errors)
	}

	// Prepare the helm template command
	templateCmd := exec.Command("helm", "template", releaseName, chartPath)

	// Add each values file to the command arguments
	for _, valuesFile := range valuesFiles {
		templateCmd.Args = append(templateCmd.Args, "--values", valuesFile)
	}

	// Buffers to capture the standard output and error streams
	var templateStdout, templateStderr bytes.Buffer
	templateCmd.Stdout = &templateStdout
	templateCmd.Stderr = &templateStderr

	// Run the Helm template command
	if err := templateCmd.Run(); err != nil {
		// If there is an error, print stderr and return the error
		return fmt.Errorf("error running helm template: %v\nstderr: %s", err, templateStderr.String())
	}

	// Output the result based on the outputFile argument
	if outputFile == "" {
		// Print the result to stdout if no outputFile is specified
		fmt.Println(templateStdout.String())
	} else {
		// Open the output file in append mode (create if it doesn't exist)
		file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error opening output file %s: %v", outputFile, err)
		}
		defer file.Close()

		// Append the rendered chart output to the file
		if _, err := file.Write(templateStdout.Bytes()); err != nil {
			return fmt.Errorf("error writing to output file %s: %v", outputFile, err)
		}

		if _, err := file.Write([]byte("\n")); err != nil {
			return fmt.Errorf("error writing separator to output file %s: %v", outputFile, err)
		}
	}

	// Defer cleanup of dependencies after helm template execution
	defer cleanupDependencies(chartPath)

	return nil
}

// Helper function to check if the release name is valid
func isValidReleaseName(name string) bool {
	// This is the Helm regex for valid release names
	const releaseNamePattern = `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	re := regexp.MustCompile(releaseNamePattern)
	return re.MatchString(name)
}

// Handle Helm chart dependencies
func handleDependencies(chartPath string) (bool, []string) {
	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	hasDependencies, err := checkForDependencies(chartYamlPath)

	// If there is an error reading the Chart.yaml file, return failure and the error message
	if err != nil {
		return false, []string{fmt.Sprintf("Error reading Chart.yaml: %v", err)}
	}

	// If the chart has dependencies, update them using Helm
	if hasDependencies {
		cacheDir, err := os.MkdirTemp("", "chartscan")
		// If there is an error creating a temp cache dir, return failure and the error message
		if err != nil {
			return false, []string{fmt.Sprintf("Error creating temp cache dir: %v", err)}
		}
		// Defer removal of the cache dir until after the function has finished
		defer os.RemoveAll(cacheDir)

		dependencyCmd := exec.Command("helm", "dependency", "update", "--repository-cache", cacheDir, chartPath)
		// If there is an error running the Helm dependency update command, return failure and the error message
		if err := dependencyCmd.Run(); err != nil {
			return false, []string{fmt.Sprintf("Error updating dependencies: %v", err)}
		}
	}

	// Return success and no errors
	return true, nil
}

// cleanupDependencies removes Helm chart dependencies and the lock file.
//
// This function takes the path to a Helm chart and removes the 'charts' directory
// and the 'Chart.lock' file associated with the chart. This cleanup is typically
// performed after updating Helm dependencies to ensure no stale or unused files remain.
func cleanupDependencies(chartPath string) {
	// Define the path to the 'charts' directory within the chart path
	chartsDir := filepath.Join(chartPath, "charts")
	// Define the path to the 'Chart.lock' file
	chartLockFile := filepath.Join(chartPath, "Chart.lock")

	// Defer the removal of the 'charts' directory and 'Chart.lock' file until the function exits
	defer func() {
		// Remove the 'charts' directory
		os.RemoveAll(chartsDir)
		// Remove the 'Chart.lock' file
		os.Remove(chartLockFile)
	}()
}

// checkValuesFilesExistence checks if the given values files exist.
// The function takes a slice of values files as input and returns a slice of errors.
// If a values file does not exist, an error message is added to the slice.
func checkValuesFilesExistence(valuesFiles []string) []string {
	var errors []string
	for _, valuesFile := range valuesFiles {
		if _, err := os.Stat(valuesFile); os.IsNotExist(err) {
			// If a values file does not exist, add an error message to the slice
			errors = append(errors, fmt.Sprintf("Values file does not exist: %s", valuesFile))
		}
	}
	return errors
}

// lintChart lints a Helm chart with the given values files.
// It returns a slice of error messages if the linting fails.
func lintChart(chartPath string, valuesFiles []string) []string {
	// Prepare the Helm lint command with strict mode
	lintCmd := exec.Command("helm", "lint", "--strict", chartPath)

	// Add each values file to the command arguments
	for _, valuesFile := range valuesFiles {
		lintCmd.Args = append(lintCmd.Args, "--values", valuesFile)
	}

	// Buffers to capture the standard output and error streams
	var lintStdout, lintStderr bytes.Buffer
	lintCmd.Stdout = &lintStdout
	lintCmd.Stderr = &lintStderr

	// Run the Helm lint command
	if err := lintCmd.Run(); err != nil {
		// Concatenate output from both stdout and stderr
		output := lintStdout.String() + lintStderr.String()
		// Parse and return error messages from the output
		return parseErrorLogs(output)
	}

	// Return nil if linting is successful
	return nil
}

func parseTemplates(chartPath string) ([]models.ValueReference, []string) {
	// Initialize slices to store value references and errors
	var valueReferences []models.ValueReference
	var errors []string

	// Define the path to the templates directory
	templatesDir := filepath.Join(chartPath, "templates")

	// Check if the templates directory exists in the root of the chartPath
	info, err := os.Stat(templatesDir)
	if os.IsNotExist(err) {
		// If the templates directory does not exist, return empty results
		return valueReferences, errors
	}
	if err != nil {
		// If there is an error accessing the templates directory, return the error
		errors = append(errors, fmt.Sprintf("Error accessing templates directory: %v", err))
		return valueReferences, errors
	}

	// Ensure the path is a directory
	if !info.IsDir() {
		errors = append(errors, fmt.Sprintf("Expected templates to be a directory but found a file: %s", templatesDir))
		return valueReferences, errors
	}

	// Walk through all files in the templates directory
	err = filepath.Walk(templatesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Append an error if there is an issue accessing a file
			errors = append(errors, fmt.Sprintf("Error accessing file %s: %v", path, err))
			return nil
		}

		// Process only YAML files
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
			// Parse the template file and extract value references
			refs, err := TemplateParser(path)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Error parsing template file %s: %v", path, err))
			} else {
				valueReferences = append(valueReferences, refs...)
			}
		}
		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("Error walking templates directory: %v", err))
	}

	// Return the value references and errors
	return valueReferences, errors
}

// Load and merge values files
//
// This function loads the chart's values.yaml file and a list of additional values files,
// and merges them together into a single map of values. If there is an issue loading
// any of the files, it returns an error.
func loadAndMergeValues(chartPath string, valuesFiles []string) (map[string]interface{}, []string) {
	// Load the chart's values.yaml file
	values, err := ValuesLoader(filepath.Join(chartPath, "values.yaml"))
	if err != nil {
		// Return an error if there is an issue loading the values.yaml file
		return nil, []string{fmt.Sprintf("Error loading values file: %v", err)}
	}

	// Initialize the values map if it is not already set
	if values == nil {
		values = make(map[string]interface{})
	}

	// Initialize a slice of errors
	var errors []string

	// Iterate over each additional values file
	for _, valuesFile := range valuesFiles {
		// Skip the chart's values.yaml file if it is in the list of additional values files
		if valuesFile != filepath.Join(chartPath, "values.yaml") {
			// Load the additional values file
			additionalValues, err := ValuesLoader(valuesFile)
			if err != nil {
				// Append an error message to the errors slice if there is an issue loading the additional values file
				errors = append(errors, fmt.Sprintf("Error loading additional values file %s: %v", valuesFile, err))
			} else {
				// Merge the additional values into the main values map
				mergeMaps(values, additionalValues)
			}
		}
	}

	// Return the merged values map and any errors that occurred
	return values, errors
}

// checkForDependencies checks if a chart has dependencies
//
// This function takes the path to a chart's Chart.yaml file as input and returns a boolean indicating
// whether the chart has dependencies and an error if there is an issue reading the Chart.yaml file.
func checkForDependencies(chartYamlPath string) (bool, error) {
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		// Return an error if there is an issue reading the Chart.yaml file
		return false, err
	}

	// Unmarshal the Chart.yaml file into a map
	var chartData map[string]interface{}
	err = yaml.Unmarshal(data, &chartData)
	if err != nil {
		// Return an error if there is an issue unmarshaling the Chart.yaml file
		return false, err
	}

	// Check if the chart has dependencies
	dependencies, ok := chartData["dependencies"]
	if !ok {
		// Return false if the chart does not have dependencies
		return false, nil
	}

	// Check if the dependencies are a slice
	depsList, ok := dependencies.([]interface{})
	if !ok || len(depsList) == 0 {
		// Return false if the dependencies are not a slice or if there are no dependencies
		return false, nil
	}

	// Return true if the chart has dependencies
	return true, nil
}

// parseErrorLogs parses the output of a Helm command and extracts any error messages.
// The function takes a string as input and returns a slice of strings containing the error messages.
func parseErrorLogs(output string) []string {
	var errorMessages []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Check if the line contains the "[ERROR]" keyword
		if strings.Contains(line, "[ERROR]") {
			// Append the line to the error messages slice if it contains an error message
			errorMessages = append(errorMessages, line)
		}
	}
	// Return the error messages slice
	return errorMessages
}

// colorSymbol returns a colored symbol string based on the success parameter.
// If the success parameter is true, the symbol is colored green, otherwise it is colored red.
func colorSymbol(s string, success bool) string {
	if success {
		return color.GreenString(s)
	} else {
		return color.RedString(s)
	}
}

func colorize(s string, color string) string {
	switch color {
	case "green":
		return "\033[32m" + s + "\033[0m"
	case "red":
		return "\033[31m" + s + "\033[0m"
	default:
		return s
	}
}

// PrintResultsPretty prints the results of a Helm chart scan in a pretty table format.
// It takes a slice of models.Result objects as input and prints the chart path, success status,
// and any error messages for each chart.
func PrintResultsPretty(results []models.Result, duration time.Duration) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Chart Name", "Success", "Details"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)
	table.SetRowLine(true)

	// Initialize counters for the number of valid and invalid charts
	var validCharts, invalidCharts int
	// Initialize a slice of rows for the table
	var rows [][]string

	// Iterate over the results and construct the table rows
	for _, result := range results {
		// Get the chart name from the Chart.yaml file
		chartName, err := getChartName(result.ChartPath)
		if err != nil {
			// If there is an error reading the Chart.yaml, fallback to the chart path
			chartName = result.ChartPath
		}

		// Set the success string to a colored checkmark or exclamation mark
		successStr := colorSymbol("✔", result.Success)
		if !result.Success {
			successStr = colorSymbol("✘", result.Success)
			invalidCharts++
		} else {
			validCharts++
		}

		// Create the row for the table
		row := []string{
			chartName,                         // Chart name
			successStr,                        // Success status
			strings.Join(result.Errors, "\n"), // Error details
		}
		rows = append(rows, row)
	}

	// Print the table rows
	for _, row := range rows {
		table.Append(row)
	}

	// Print the table
	table.Render()

	// Print the summary
	fmt.Printf("\nSummary: %d valid charts, %d invalid charts scanned in %v\n", validCharts, invalidCharts, duration)
}

// getChartName reads the Chart.yaml file from the given chart path and returns the chart name.
func getChartName(chartPath string) (string, error) {
	// Construct the path to the Chart.yaml file
	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")

	// Read the Chart.yaml file
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		return "", fmt.Errorf("error reading Chart.yaml: %v", err)
	}

	// Parse the YAML content of Chart.yaml
	var chartData map[string]interface{}
	err = yaml.Unmarshal(data, &chartData)
	if err != nil {
		return "", fmt.Errorf("error parsing Chart.yaml: %v", err)
	}

	// Extract the chart name
	name, ok := chartData["name"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid name in Chart.yaml")
	}

	return name, nil
}
