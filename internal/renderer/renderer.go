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
	matches := re.FindAllStringSubmatchIndex(templateString, -1)

	lines := strings.Split(templateString, "\n")
	for _, match := range matches {
		lineNum := 1
		for i, line := range lines {
			if strings.Contains(line, templateString[match[0]:match[1]]) {
				lineNum = i + 1
				break
			}
		}

		reference := templateString[match[2]:match[3]]
		valueReference := models.ValueReference{
			Name:     reference,
			File:     templateFile,
			Line:     lineNum,
			FullText: templateString[match[0]:match[1]],
		}
		valueReferences = append(valueReferences, valueReference)
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
	var undefinedValues []string

	for _, valueReference := range valueReferences {
		keys := strings.Split(valueReference.Name, ".")

		if !checkNestedValueExists(keys, values) {
			// Add detailed info when the value is undefined
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
		if nextMap, exists := v[keys[0]]; exists {
			return checkNestedValueExists(keys[1:], nextMap)
		}

		return false // Key not found at this level

	default:
		return false // If the current value is not a map, return false
	}
}

// Merge maps, merging nested maps recursively.
func mergeMaps(target, source map[string]interface{}) {
	for key, value := range source {
		if targetValue, exists := target[key]; exists {
			// If the key already exists, and both are maps, recursively merge
			if targetMap, ok := targetValue.(map[string]interface{}); ok {
				if sourceMap, ok := value.(map[string]interface{}); ok {
					mergeMaps(targetMap, sourceMap)
					continue
				}
			}
		}
		// If not a map or doesn't exist, just overwrite the target value
		target[key] = value
	}
}

func RenderHelmChart(chartPath string, valuesFiles []string) (bool, []string, map[string]interface{}, []string) {
	if chartPath == "" {
		return false, []string{"Chart path is empty"}, nil, nil
	}

	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	hasDependencies, err := checkForDependencies(chartYamlPath)
	if err != nil {
		return false, []string{fmt.Sprintf("Error reading Chart.yaml: %v", err)}, nil, nil
	}
	if hasDependencies {
		if cacheDir, err := os.MkdirTemp("", "chartscan"); err != nil {
			return false, []string{fmt.Sprintf("Error creating temp cache dir: %v", err)}, nil, nil
		} else {
			defer os.RemoveAll(cacheDir)
			dependencyCmd := exec.Command("helm", "dependency", "update", "--repository-cache", cacheDir, chartPath)
			var dependencyStderr bytes.Buffer
			dependencyCmd.Stderr = &dependencyStderr
			dependencyCmd.Stdout = &bytes.Buffer{}

			if err := dependencyCmd.Run(); err != nil {
				return false, []string{fmt.Sprintf("Error updating dependencies: %v\n%s", err, dependencyStderr.String())}, nil, nil
			}
		}


		// Cleanup fetched Helm dependencies
		chartsDir := filepath.Join(chartPath, "charts")
		chartLockFile := filepath.Join(chartPath, "Chart.lock")
		defer func() {
			if err := os.RemoveAll(chartsDir); err != nil {
				fmt.Printf("Warning: Failed to clean up charts directory: %v\n", err)
			}
			if err := os.Remove(chartLockFile); err != nil && !os.IsNotExist(err) {
				fmt.Printf("Warning: Failed to remove Chart.lock: %v\n", err)
			}
		}()
	}

	// Check if each values file exists and exit immediately if any does not exist
	var missingValuesFiles []string
	for _, valuesFile := range valuesFiles {
		if _, err := os.Stat(valuesFile); os.IsNotExist(err) {
			missingValuesFiles = append(missingValuesFiles, valuesFile)
		}
	}

	// If there are missing values files, exit immediately and return an error message
	if len(missingValuesFiles) > 0 {
		var errors []string
		for _, file := range missingValuesFiles {
			errors = append(errors, fmt.Sprintf("Values file does not exist: %s", file))
		}
		fmt.Println(strings.Join(errors, "\n"))
		os.Exit(1) // Exit the program with a non-zero status
	}

	// Lint the chart
	lintCmd := exec.Command("helm", "lint", "--strict", chartPath)
	for _, valuesFile := range valuesFiles {
		lintCmd.Args = append(lintCmd.Args, "--values", valuesFile)
	}

	var lintStdout, lintStderr bytes.Buffer
	lintCmd.Stdout = &lintStdout
	lintCmd.Stderr = &lintStderr

	var lintErrors []string
	if err := lintCmd.Run(); err != nil {
		output := lintStdout.String() + lintStderr.String()
		lintErrors = parseErrorLogs(output)
	}

	// Parse templates and check value references
	templateFiles, err := filepath.Glob(filepath.Join(chartPath, "templates", "*.yaml"))
	if err != nil {
		return false, append(lintErrors, fmt.Sprintf("Error finding template files: %v", err)), nil, nil
	}

	var valueReferences []models.ValueReference
	for _, templateFile := range templateFiles {
		refs, err := TemplateParser(templateFile)
		if err != nil {
			return false, append(lintErrors, fmt.Sprintf("Error parsing template file: %v", err)), nil, nil
		}
		valueReferences = append(valueReferences, refs...)
	}

	// Load the default values file (values.yaml)
	values, err := ValuesLoader(filepath.Join(chartPath, "values.yaml"))
	if err != nil {
		return false, append(lintErrors, fmt.Sprintf("Error loading values file: %v", err)), nil, nil
	}

	// Ensure that values map is initialized if it is nil
	if values == nil {
		values = make(map[string]interface{})
	}

	// Merge additional values files
	for _, valuesFile := range valuesFiles {
		if valuesFile != filepath.Join(chartPath, "values.yaml") {
			additionalValues, err := ValuesLoader(valuesFile)
			if err != nil {
				return false, append(lintErrors, fmt.Sprintf("Error loading additional values file %s: %v", valuesFile, err)), nil, nil
			}
			// Ensure that additional values map is initialized if it is nil
			if additionalValues == nil {
				additionalValues = make(map[string]interface{})
			}
			// Merge the additional values into the primary values map
			mergeMaps(values, additionalValues)
		}
	}

	// Check for undefined values
	var undefinedValues []string
	if len(valueReferences) > 0 {
		undefinedValues = CheckValueReferences(valueReferences, values)
	}

	// Combine errors
	allErrors := append(lintErrors, undefinedValues...)

	// Success depends on the presence of errors
	success := len(allErrors) == 0

	return success, allErrors, values, undefinedValues
}

func checkForDependencies(chartYamlPath string) (bool, error) {
	file, err := os.Open(chartYamlPath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	var chartData map[string]interface{}
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&chartData)
	if err != nil {
		return false, err
	}
	dependencies, ok := chartData["dependencies"]
	if !ok {
		return false, nil
	}
	depsList, ok := dependencies.([]interface{})
	if !ok {
		return false, fmt.Errorf("dependencies field in Chart.yaml is not a list")
	}
	if len(depsList) == 0 {
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

	for _, result := range results {
		successStr := colorSymbol("✔", result.Success)
		if !result.Success {
			successStr = colorSymbol("✘", result.Success)
			invalidCharts++
		} else {
			validCharts++
		}

		errorStr := ""
		if len(result.Errors) > 0 {
			errorStr += "Errors:\n"
			for _, err := range result.Errors {
				errorStr += "* " + err + "\n"
			}
		}

		table.Append([]string{
			result.ChartPath,
			successStr,
			errorStr,
		})
	}

	table.Render()

	// Summary Table
	summaryTable := tablewriter.NewWriter(os.Stdout)
	summaryTable.SetHeader([]string{"Category", "Count"})
	summaryTable.Append([]string{"Valid Charts", colorize(strconv.Itoa(validCharts), "green")})
	summaryTable.Append([]string{"Invalid Charts", colorize(strconv.Itoa(invalidCharts), "red")})
	summaryTable.Render()
}
