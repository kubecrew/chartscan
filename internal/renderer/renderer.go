package renderer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"

	"github.com/Jaydee94/chartscan/internal/models"
)

// TemplateParser parses a template file and extracts value references
func TemplateParser(templateFile string) ([]models.ValueReference, error) {
	templateBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}

	templateString := string(templateBytes)
	var valueReferences []models.ValueReference

	// Regex to capture dot notation values like .Values.service.port
	re := regexp.MustCompile(`{{\s*\.Values\.([a-zA-Z0-9_.\[\]-]+)\s*}}`)
	lines := strings.Split(templateString, "\n")
	for i, line := range lines {
		matches := re.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			reference := line[match[2]:match[3]]
			if reference == "" {
				return nil, fmt.Errorf("empty value reference: %s", line[match[0]:match[1]])
			}

			valueReference := models.ValueReference{
				Name:     reference,
				File:     templateFile,
				Line:     i + 1,
				FullText: line[match[0]:match[1]],
			}
			valueReferences = append(valueReferences, valueReference)
		}
	}
	return valueReferences, nil
}

// ValuesLoader loads values from a YAML file
func ValuesLoader(valuesFile string) (map[string]interface{}, error) {
	valuesBytes, err := os.ReadFile(valuesFile)
	if err != nil {
		return nil, err
	}

	var values map[string]interface{}
	err = yaml.Unmarshal(valuesBytes, &values)
	if err != nil {
		return nil, err
	}

	return values, nil
}

func CheckValueReferences(valueReferences []models.ValueReference, values map[string]interface{}) []string {
	undefinedValues := make([]string, 0, len(valueReferences))

	for _, valueReference := range valueReferences {
		keys := strings.Split(valueReference.Name, ".")
		if !checkNestedValueExists(keys, values) {
			undefinedValues = append(undefinedValues, fmt.Sprintf("Undefined value: '%s' referenced in %s at line %d", valueReference.Name, valueReference.File, valueReference.Line))
		}
	}

	return undefinedValues
}

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

// Merge maps, merging nested maps recursively and avoiding unnecessary type assertions.
func mergeMaps(target, source map[string]interface{}) {
	for key, value := range source {
		if targetMap, ok := target[key].(map[string]interface{}); ok {
			if sourceMap, ok := value.(map[string]interface{}); ok {
				mergeMaps(targetMap, sourceMap)
				continue
			}
		}
		target[key] = value
	}
}

// Refactored RenderHelmChart function and its components

func RenderHelmChart(chartPath string, valuesFiles []string) (bool, []string, map[string]interface{}, []string) {
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

	return success, allErrors, values, undefinedValues
}

// Handle Helm chart dependencies
func handleDependencies(chartPath string) (bool, []string) {
	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	hasDependencies, err := checkForDependencies(chartYamlPath)
	if err != nil {
		return false, []string{fmt.Sprintf("Error reading Chart.yaml: %v", err)}
	}
	if hasDependencies {
		cacheDir, err := os.MkdirTemp("", "chartscan")
		if err != nil {
			return false, []string{fmt.Sprintf("Error creating temp cache dir: %v", err)}
		}
		defer os.RemoveAll(cacheDir)

		dependencyCmd := exec.Command("helm", "dependency", "update", "--repository-cache", cacheDir, chartPath)
		if err := dependencyCmd.Run(); err != nil {
			return false, []string{fmt.Sprintf("Error updating dependencies: %v", err)}
		}

		// Cleanup fetched dependencies
		cleanupDependencies(chartPath)
	}
	return true, nil
}

// Cleanup Helm dependencies
func cleanupDependencies(chartPath string) {
	chartsDir := filepath.Join(chartPath, "charts")
	chartLockFile := filepath.Join(chartPath, "Chart.lock")
	defer func() {
		os.RemoveAll(chartsDir)
		os.Remove(chartLockFile)
	}()
}

// Check if values files exist
func checkValuesFilesExistence(valuesFiles []string) []string {
	var errors []string
	for _, valuesFile := range valuesFiles {
		if _, err := os.Stat(valuesFile); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("Values file does not exist: %s", valuesFile))
		}
	}
	return errors
}

// Lint the chart
func lintChart(chartPath string, valuesFiles []string) []string {
	lintCmd := exec.Command("helm", "lint", "--strict", chartPath)
	for _, valuesFile := range valuesFiles {
		lintCmd.Args = append(lintCmd.Args, "--values", valuesFile)
	}

	var lintStdout, lintStderr bytes.Buffer
	lintCmd.Stdout = &lintStdout
	lintCmd.Stderr = &lintStderr

	if err := lintCmd.Run(); err != nil {
		output := lintStdout.String() + lintStderr.String()
		return parseErrorLogs(output)
	}
	return nil
}

// Parse templates and gather value references
func parseTemplates(chartPath string) ([]models.ValueReference, []string) {
	templateFiles, err := filepath.Glob(filepath.Join(chartPath, "templates", "*.yaml"))
	if err != nil {
		return nil, []string{fmt.Sprintf("Error finding template files: %v", err)}
	}

	var valueReferences []models.ValueReference
	var errors []string
	for _, templateFile := range templateFiles {
		refs, err := TemplateParser(templateFile)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error parsing template file %s: %v", templateFile, err))
		} else {
			valueReferences = append(valueReferences, refs...)
		}
	}
	return valueReferences, errors
}

// Load and merge values files
func loadAndMergeValues(chartPath string, valuesFiles []string) (map[string]interface{}, []string) {
	values, err := ValuesLoader(filepath.Join(chartPath, "values.yaml"))
	if err != nil {
		return nil, []string{fmt.Sprintf("Error loading values file: %v", err)}
	}

	if values == nil {
		values = make(map[string]interface{})
	}

	var errors []string
	for _, valuesFile := range valuesFiles {
		if valuesFile != filepath.Join(chartPath, "values.yaml") {
			additionalValues, err := ValuesLoader(valuesFile)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Error loading additional values file %s: %v", valuesFile, err))
			} else {
				mergeMaps(values, additionalValues)
			}
		}
	}
	return values, errors
}

func checkForDependencies(chartYamlPath string) (bool, error) {
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		return false, err
	}
	var chartData map[string]interface{}
	err = yaml.Unmarshal(data, &chartData)
	if err != nil {
		return false, err
	}
	dependencies, ok := chartData["dependencies"]
	if !ok {
		return false, nil
	}
	depsList, ok := dependencies.([]interface{})
	if !ok || len(depsList) == 0 {
		return false, nil
	}
	return true, nil
}

func parseErrorLogs(output string) []string {
	var errorMessages []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "[ERROR]") {
			errorMessages = append(errorMessages, line)
		}
	}
	return errorMessages
}

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

func PrintResultsPretty(results []models.Result) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Chart Path", "Success", "Details"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)
	table.SetRowLine(true)

	var validCharts, invalidCharts int
	var rows [][]string

	for _, result := range results {
		successStr := colorSymbol("✔", result.Success)
		if !result.Success {
			successStr = colorSymbol("✘", result.Success)
			invalidCharts++
		} else {
			validCharts++
		}

		var errorStr strings.Builder
		if len(result.Errors) > 0 {
			errorStr.WriteString("Errors:\n")
			for _, err := range result.Errors {
				errorStr.WriteString("* " + err + "\n")
			}
		}

		rows = append(rows, []string{
			result.ChartPath,
			successStr,
			errorStr.String(),
		})
	}

	table.AppendBulk(rows)
	table.Render()

	// Summary Table
	summaryTable := tablewriter.NewWriter(os.Stdout)
	summaryTable.SetHeader([]string{"Category", "Count"})
	summaryTable.AppendBulk([][]string{
		{"Valid Charts", colorize(strconv.Itoa(validCharts), "green")},
		{"Invalid Charts", colorize(strconv.Itoa(invalidCharts), "red")},
	})
	summaryTable.Render()
}
