package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Jaydee94/chartscan/internal/finder"
	"github.com/Jaydee94/chartscan/internal/models"
	"github.com/Jaydee94/chartscan/internal/renderer"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Config represents the structure of the YAML configuration file.
type Config struct {
	ChartPath   string   `yaml:"chartPath"`
	ValuesFiles []string `yaml:"valuesFiles"`
	Format      string   `yaml:"format"`
}

// checkHelmInstalled verifies if the Helm binary is available.
func checkHelmInstalled() error {
	_, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm binary is not installed or not in your PATH. Please install Helm to proceed")
	}
	return nil
}

func main() {
	var format string
	var configFile string

	rootCmd := &cobra.Command{
		Use:   "chartscan [chart-path]",
		Short: "ChartScan is a tool to scan Helm charts",
		Run: func(cmd *cobra.Command, args []string) {
			// Check if Helm is installed
			if err := checkHelmInstalled(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			var config Config

			// Load configuration file if provided
			if configFile != "" {
				file, err := os.Open(configFile)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error opening config file: %v\n", err)
					os.Exit(1)
				}
				defer file.Close()

				decoder := yaml.NewDecoder(file)
				if err := decoder.Decode(&config); err != nil {
					fmt.Fprintf(os.Stderr, "Error decoding config file: %v\n", err)
					os.Exit(1)
				}
			}

			// Override config values with flags if provided
			if len(args) > 0 {
				config.ChartPath = args[0]
			}
			if config.ChartPath == "" {
				cmd.Help()
				os.Exit(1)
			}

			valuesFiles, err := cmd.Flags().GetStringSlice("values")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting values files: %v\n", err)
				os.Exit(1)
			}
			if len(valuesFiles) > 0 {
				config.ValuesFiles = valuesFiles
			}
			if config.ValuesFiles == nil {
				config.ValuesFiles = []string{}
			}

			if format != "" {
				config.Format = format
			}
			if config.Format == "" {
				config.Format = "pretty"
			}

			chartDirs, err := finder.FindHelmChartDirs(config.ChartPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error finding Helm charts: %v\n", err)
				os.Exit(1)
			}

			s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
			s.Start()
			defer s.Stop()

			var scanResults []models.Result
			var invalidCharts int // Counter for invalid charts

			for _, chartDir := range chartDirs {
				s.Suffix = fmt.Sprintf(" Scanning charts: %s", chartDirs)
				success, errors, values, undefinedValues := renderer.RenderHelmChart(chartDir, config.ValuesFiles)
				if !success {
					invalidCharts++ // Increment invalid charts counter
				}
				scanResults = append(scanResults, models.Result{
					ChartPath:       chartDir,
					Success:         success,
					Errors:          errors,
					Values:          values,
					UndefinedValues: undefinedValues,
				})
			}

			s.Stop()

			// Print results based on the format
			switch config.Format {
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
				fmt.Fprintf(os.Stderr, "Unknown output format: %s\n", config.Format)
				os.Exit(1)
			}

			// Set exit code based on whether there are invalid charts
			if invalidCharts > 0 {
				os.Exit(1) // Exit with code 1 if there are invalid charts
			}
		},
	}

	// Define command-line flags
	rootCmd.Flags().StringSliceP("values", "f", []string{}, "Values files to use for rendering")
	rootCmd.Flags().StringVarP(&format, "format", "o", "pretty", "Output format (pretty, json, yaml)")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}
