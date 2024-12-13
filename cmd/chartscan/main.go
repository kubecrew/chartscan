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
	var format string

	rootCmd := &cobra.Command{
		Use:   "chartscan [chart-path]",
		Short: "ChartScan is a tool to scan Helm charts",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 1 {
				cmd.Help()
				os.Exit(1)
			}

			chartPath := args[0]
			valuesFiles, err := cmd.Flags().GetStringSlice("values")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting values files: %v\n", err)
				os.Exit(1)
			}

			if len(valuesFiles) == 0 {
				valuesFiles = []string{}
			}

			chartDirs, err := finder.FindHelmChartDirs(chartPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error finding Helm charts: %v\n", err)
				os.Exit(1)
			}

			var scanResults []models.Result
			for _, chartDir := range chartDirs {
				success, errors, values, undefinedValues := renderer.RenderHelmChart(chartDir, valuesFiles)
				scanResults = append(scanResults, models.Result{
					ChartPath:       chartDir,
					Success:         success,
					Errors:          errors,
					Values:          values,
					UndefinedValues: undefinedValues,
				})
			}

			switch format {
			case "pretty":
				renderer.PrintResultsPretty(scanResults)
			case "json":
				output, err := json.MarshalIndent(scanResults, "", "  ")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error marshaling results to JSON: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(string(output))
			case "yaml":
				output, err := yaml.Marshal(scanResults)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error marshaling results to YAML: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(string(output))
			default:
				fmt.Fprintf(os.Stderr, "Unknown output format: %s\n", format)
				os.Exit(1)
			}
		},
	}

	rootCmd.Flags().StringSliceP("values", "f", []string{}, "values files to use for rendering")
	rootCmd.Flags().StringVarP(&format, "format", "o", "pretty", "output format (pretty, json, yaml)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}
