package renderer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"

	"github.com/Jaydee94/chartscan/internal/models"
)

func RenderHelmChart(chartPath string) (bool, []string) {
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

		err := dependencyCmd.Run()
		if err != nil {
			return false, []string{fmt.Sprintf("Error updating dependencies: %v\n%s", err, dependencyStderr.String())}
		}
	}
	lintCmd := exec.Command("helm", "lint", chartPath)
	var lintStdout, lintStderr bytes.Buffer
	lintCmd.Stdout = &lintStdout
	lintCmd.Stderr = &lintStderr

	err = lintCmd.Run()
	if err != nil {
		output := lintStdout.String() + lintStderr.String()
		errorLogs := parseErrorLogs(output)
		return false, errorLogs
	}
	return true, []string{}
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
	if dependencies, ok := chartData["dependencies"]; ok {
		if depsList, ok := dependencies.([]interface{}); ok && len(depsList) > 0 {
			return true, nil
		}
	}
	return false, nil
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

func PrintResultsPretty(results []models.Result) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Chart Path", "Success", "Errors"})
	table.SetRowLine(true)

	successColor := func(s string) string {
		return color.GreenString(s)
	}
	failureColor := func(s string) string {
		return color.RedString(s)
	}

	for _, result := range results {
		var successStr string
		if result.Success {
			successStr = successColor("✔")
		} else {
			successStr = failureColor("✘")
		}

		errorStr := ""
		if len(result.Errors) > 0 {
			errorLines := make([]string, len(result.Errors))
			for i, err := range result.Errors {
				errorLines[i] = failureColor(err)
			}
			errorStr = strings.Join(errorLines, "\n")
		}

		table.Append([]string{
			result.ChartPath,
			successStr,
			errorStr,
		})
	}

	table.Render()
}
