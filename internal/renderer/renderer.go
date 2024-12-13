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

type ValueReference struct {
	Name string
}

// TemplateParser parses a template file and extracts value references
func TemplateParser(templateFile string) ([]ValueReference, error) {
	templateBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}

	templateString := string(templateBytes)
	var valueReferences []ValueReference

	// Use regular expressions to extract value references from the template
	re := regexp.MustCompile(`{{\s*([a-zA-Z0-9_]+)\s*}}`)
	matches := re.FindAllStringSubmatch(templateString, -1)

	for _, match := range matches {
		valueReference := ValueReference{
			Name: match[1],
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

// CheckValueReferences checks if value references are defined in the values file
func CheckValueReferences(valueReferences []ValueReference, values map[string]interface{}) []string {
	var undefinedValues []string
	for _, valueReference := range valueReferences {
		if _, ok := values[valueReference.Name]; !ok {
			undefinedValues = append(undefinedValues, valueReference.Name)
		}
	}
	return undefinedValues
}

func RenderHelmChart(chartPath string, valuesFiles []string) (bool, []string) {
	if chartPath == "" {
		return false, []string{"Chart path is empty"}
	}

	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	hasDependencies, err := checkForDependencies(chartYamlPath)
	if err != nil {
		return false, []string{fmt.Sprintf("Error reading Chart.yaml: %v", err)}
	}
	if hasDependencies {
		dependencyCmd := exec.Command("helm", "dependency", "update", chartPath)
		var dependencyStderr bytes.Buffer
		dependencyCmd.Stderr = &dependencyStderr
		dependencyCmd.Stdout = &bytes.Buffer{}

		if err := dependencyCmd.Run(); err != nil {
			return false, []string{fmt.Sprintf("Error updating dependencies: %v\n%s", err, dependencyStderr.String())}
		}
	}

	lintCmd := exec.Command("helm", "lint", "--strict", chartPath)
	for _, valuesFile := range valuesFiles {
		lintCmd.Args = append(lintCmd.Args, "--values", valuesFile)
	}

	var lintStdout, lintStderr bytes.Buffer
	lintCmd.Stdout = &lintStdout
	lintCmd.Stderr = &lintStderr

	if err := lintCmd.Run(); err != nil {
		output := lintStdout.String() + lintStderr.String()
		errorLogs := parseErrorLogs(output)
		return false, errorLogs
	}

	// Check value references in templates
	templateFiles, err := filepath.Glob(filepath.Join(chartPath, "templates", "*.yaml"))
	if err != nil {
		return false, []string{fmt.Sprintf("Error finding template files: %v", err)}
	}

	for _, templateFile := range templateFiles {
		valueReferences, err := TemplateParser(templateFile)
		if err != nil {
			return false, []string{fmt.Sprintf("Error parsing template file: %v", err)}
		}

		values, err := ValuesLoader(filepath.Join(chartPath, "values.yaml"))
		if err != nil {
			return false, []string{fmt.Sprintf("Error loading values file: %v", err)}
		}

		CheckValueReferences(valueReferences, values)
	}

	return true, nil
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
	table.SetHeader([]string{"Chart Path", "Success", "Errors"})
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)
	table.SetRowLine(true)

	var validCharts int
	var invalidCharts int

	for _, result := range results {
		var successStr string
		if result.Success {
			successStr = colorSymbol("✔", true)
			validCharts++
		} else {
			successStr = colorSymbol("✘", false)
			invalidCharts++
		}

		errorStr := ""
		if len(result.Errors) > 0 {
			errorLines := make([]string, len(result.Errors))
			for i, err := range result.Errors {
				errorLines[i] = "* " + err
			}
			errorStr = strings.Join(errorLines, "\n")
			table.Append([]string{
				result.ChartPath,
				successStr,
				errorStr,
			})
		} else {
			table.Append([]string{
				result.ChartPath,
				successStr,
				"",
			})
		}
	}
	table.Render()
	summaryTable := tablewriter.NewWriter(os.Stdout)
	summaryTable.SetHeader([]string{"Category", "Count"})
	summaryTable.Append([]string{"Valid Charts", colorize(strconv.Itoa(validCharts), "green")})
	summaryTable.Append([]string{"Invalid Charts", colorize(strconv.Itoa(invalidCharts), "red")})
	summaryTable.Render()
}
