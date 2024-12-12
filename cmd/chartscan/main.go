package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/Jaydee94/chartscan/internal/finder"
	"github.com/Jaydee94/chartscan/internal/models"
	"github.com/Jaydee94/chartscan/internal/renderer"
	"github.com/spf13/cobra"
)

func main() {
	var rootPath string
	var outputFormat string

	var rootCmd = &cobra.Command{
		Use:   "chartscan",
		Short: "ChartScan is a tool to scan Helm charts",
		Run: func(cmd *cobra.Command, args []string) {
			if rootPath == "" && len(args) < 1 {
				cmd.Help()
				os.Exit(1)
			}

			if rootPath == "" && len(args) > 0 {
				rootPath = args[0]
			}

			chartDirs, err := finder.FindHelmChartDirs(rootPath)
			if err != nil {
				fmt.Printf("Error finding Helm charts: %v\n", err)
				os.Exit(1)
			}

			var results []models.Result
			for _, chartDir := range chartDirs {
				success, errors := renderer.RenderHelmChart(chartDir)
				results = append(results, models.Result{
					ChartPath: chartDir,
					Success:   success,
					Errors:    errors,
				})
			}

			switch outputFormat {
			case "pretty":
				renderer.PrintResultsPretty(results)
			case "json":
				output, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					fmt.Printf("Error marshaling results to JSON: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(string(output))
			case "yaml":
				output, err := yaml.Marshal(results)
				if err != nil {
					fmt.Printf("Error marshaling results to YAML: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(string(output))
			default:
				fmt.Printf("Invalid output format: %s\n", outputFormat)
				cmd.Help()
				os.Exit(1)
			}

		},
	}

	rootCmd.Flags().StringVarP(&rootPath, "path", "p", "", "Path to the Helm charts directory")
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "pretty", "Output format (json|yaml|pretty)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
