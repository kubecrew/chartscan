package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Jaydee94/chartscan/internal/finder"
	"github.com/Jaydee94/chartscan/internal/models"
	"github.com/Jaydee94/chartscan/internal/renderer"
	"github.com/spf13/cobra"
)

func main() {
	// Define a variable for the root path
	var rootPath string
	var outputFormat string

	// Create a new root command
	var rootCmd = &cobra.Command{
		Use:   "chartscan",
		Short: "ChartScan is a tool to scan Helm charts",
		Run: func(cmd *cobra.Command, args []string) {
			// Check if rootPath is provided by flag or argument
			if rootPath == "" && len(args) < 1 {
				cmd.Help() // Show help if neither is provided
				os.Exit(1)
			}

			// Use the argument if rootPath flag is not set
			if rootPath == "" && len(args) > 0 {
				rootPath = args[0]
			}

			// Find Helm chart directories
			chartDirs, err := finder.FindHelmChartDirs(rootPath)
			if err != nil {
				fmt.Printf("Error finding Helm charts: %v\n", err)
				os.Exit(1)
			}

			// Process the found chart directories
			var results []models.Result
			for _, chartDir := range chartDirs {
				success, renderError := renderer.RenderHelmChart(chartDir)
				results = append(results, models.Result{
					ChartPath: chartDir,
					Success:   success,
					Error:     renderError,
				})
			}

			// Print results based on the requested output format
			if outputFormat == "pretty" {
				// Print results in a pretty table format
				renderer.PrintResultsPretty(results)
			} else {
				// Default output as JSON
				output, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					fmt.Printf("Error marshaling results: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(string(output))
			}
		},
	}

	// Add the --path flag to specify the root path
	rootCmd.Flags().StringVarP(&rootPath, "path", "p", "", "Path to the Helm charts directory")
	// Add --output flag to specify the output format (default is JSON)
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "json", "Output format (json|pretty)")

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
