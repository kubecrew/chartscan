package renderer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"

	"github.com/Jaydee94/chartscan/internal/models"
)

// RenderHelmChart will render the Helm chart at the given chartPath, ensuring any dependencies are fetched.
func RenderHelmChart(chartPath string) (bool, string) {
	// Step 1: Run `helm dependency update` to fetch chart dependencies
	cmd := exec.Command("helm", "dependency", "update", chartPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{} // Discard standard output for the update command

	err := cmd.Run()
	if err != nil {
		// If there was an error fetching dependencies, return false with the error message
		return false, fmt.Sprintf("Error updating dependencies: %v\n%s", err, stderr.String())
	}

	// Step 2: Run `helm template` to render the chart
	cmd = exec.Command("helm", "template", chartPath)
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{} // Discard standard output for the render command

	err = cmd.Run()
	if err != nil {
		// If rendering the chart fails, return false with the error message
		return false, fmt.Sprintf("Error rendering chart: %v\n%s", err, stderr.String())
	}

	// Return success if both operations are successful
	return true, ""
}

// PrintResultsPretty formats and prints the results in a table with color-coded output
func PrintResultsPretty(results []models.Result) {
	// Create a new table writer
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Chart Path", "Success", "Error"})

	// Enable row line for better visual separation between rows
	table.SetRowLine(true)

	// Customize table separators
	table.SetCenterSeparator("*") // Change center separator between columns
	table.SetColumnSeparator("╪") // Change column separator
	table.SetRowSeparator("-")    // Change row separator between rows

	// Define the color functions
	successColor := func(s string) string {
		return color.GreenString(s) // This wraps the GreenString function
	}
	failureColor := func(s string) string {
		return color.RedString(s) // This wraps the RedString function
	}

	// Loop through results and add rows to the table
	for _, result := range results {
		var successStr string
		if result.Success {
			successStr = successColor("✔") // Green checkmark for success
		} else {
			successStr = failureColor("✘") // Red cross for failure
		}

		// Format the error message (if any) in red
		var errorMsg string
		if result.Error != "" {
			errorMsg = failureColor(result.Error)
		} else {
			errorMsg = ""
		}

		// Add the row to the table
		table.Append([]string{
			result.ChartPath,
			successStr,
			errorMsg,
		})

	}

	// Render the table
	table.Render()
}
