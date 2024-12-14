package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Jaydee94/chartscan/internal/finder"
	"github.com/Jaydee94/chartscan/internal/models"
	"github.com/Jaydee94/chartscan/internal/renderer"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Config holds the program configuration

var version = "dev"

func main() {
	// configFile stores the path to the configuration file
	var configFile string
	// valuesFiles stores the list of values files to use during rendering
	var valuesFiles []string
	// format stores the desired output format
	var format string

	// Root command
	rootCmd := &cobra.Command{
		Use:   "chartscan",
		Short: "ChartScan is a tool to scan Helm charts",
	}

	// Scan subcommand
	scanCmd := &cobra.Command{
		Use:   "scan [chart-path]",
		Short: "Scan Helm charts for potential issues",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Load the configuration from the configuration file and/or CLI arguments
			config, err := loadConfig(configFile, valuesFiles, format, args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}

			startTime := time.Now()

			// Find the Helm charts to scan
			chartDirs, err := finder.FindHelmChartDirs(config.ChartPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error finding Helm charts: %v\n", err)
				os.Exit(1)
			}

			// Process the Helm charts
			results, invalidCharts := processCharts(chartDirs, config)

			duration := time.Since(startTime)

			var output []byte
			// Output the results in the desired format
			switch config.Format {
			case "pretty":
				// Print the results in a pretty format
				renderer.PrintResultsPretty(results, duration)
			case "json":
				// Marshal the results to JSON
				output, err = json.MarshalIndent(results, "", "  ")
			case "yaml":
				// Marshal the results to YAML
				output, err = yaml.Marshal(results)
			case "junit":
				// Print JUnit test report
				err = printJUnitTestReport(results)
			default:
				fmt.Fprintf(os.Stderr, "Unknown output format: %s\n", config.Format)
				os.Exit(1)
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "Error processing results: %v\n", err)
				os.Exit(1)
			}

			if output != nil {
				// Print the output
				fmt.Println(string(output))
			}

			// Exit with a non-zero status if there are invalid charts
			if invalidCharts > 0 {
				os.Exit(1)
			}
		},
	}

	// Add flags to the scan subcommand
	scanCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	scanCmd.Flags().StringSliceVarP(&valuesFiles, "values", "f", nil, "Specify values files for rendering")
	scanCmd.Flags().StringVarP(&format, "output-format", "o", "pretty", "Output format (pretty, json, yaml, junit)")

	// Version subcommand
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of ChartScan",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("ChartScan version %s\n", version)
		},
	}

	// Add subcommands to the root command
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(versionCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// printJUnitTestReport generates a JUnit-compatible unit test report
// from the given results.
//
// The report will contain one test case per chart, with a failure
// if the chart did not render successfully.
func printJUnitTestReport(results []models.Result) error {
	var testCases []models.TestCase
	failures := 0

	for _, result := range results {
		testCase := models.TestCase{
			Name:      result.ChartPath,
			ClassName: "ChartScan",
			Time:      "0", // Dummy value for now; can measure rendering time if required
		}

		if !result.Success {
			testCase.Failure = &models.Failure{
				Message: "Chart rendering failed",
				Type:    "RenderingError",
				Content: fmt.Sprintf("Errors: %v\nUndefined Values: %v", result.Errors, result.UndefinedValues),
			}
			failures++
		} else {
			testCase.SystemOut = &models.SystemOut{
				Content: fmt.Sprintf("Chart %v rendered successfully", result.ChartPath),
			}
		}

		testCases = append(testCases, testCase)
	}

	suite := models.TestSuite{
		Name:      "Helm Chart Scan",
		Tests:     len(results),
		Failures:  failures,
		TestCases: testCases,
	}

	output, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}

// loadConfig dynamically loads the configuration from a file and/or CLI arguments
//
// The configuration is loaded from the given file (if specified) and/or
// overridden with the given CLI arguments and default values.
func loadConfig(configFile string, valuesFiles []string, format string, args []string) (models.Config, error) {
	config := models.Config{}

	// Load from configuration file if specified
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return config, fmt.Errorf("error reading config file: %v", err)
		}

		// Unmarshal the configuration from the file
		if err := yaml.Unmarshal(data, &config); err != nil {
			return config, fmt.Errorf("error decoding config file: %v", err)
		}
	}

	// Override with CLI arguments and defaults
	if len(args) > 0 {
		// Use the first command-line argument as the chart path
		config.ChartPath = args[0]
	} else if config.ChartPath == "" {
		// Default chart path
		config.ChartPath = "./charts"
	}

	if len(valuesFiles) > 0 {
		// Use the values files specified on the command line
		config.ValuesFiles = valuesFiles
	}

	if format != "" {
		// Use the output format specified on the command line
		config.Format = format
	} else if config.Format == "" {
		// Default output format
		config.Format = "pretty"
	}

	return config, nil
}

// processCharts scans and processes all chart directories concurrently
//
// This function takes a list of chart directories and a configuration object, and
// scans and processes all the charts concurrently. It returns a slice of results
// and the number of invalid charts.
func processCharts(chartDirs []string, config models.Config) ([]models.Result, int) {
	var wg sync.WaitGroup
	var mutex sync.Mutex

	results := []models.Result{}
	invalidCharts := 0

	// Create a spinner to indicate progress
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Start()
	defer s.Stop()

	// Add a wait group entry for each chart to be processed
	wg.Add(len(chartDirs))
	for _, chartDir := range chartDirs {
		go func(chartDir string) {
			defer wg.Done()

			// Update the spinner with the chart being scanned
			s.Suffix = fmt.Sprintf(" Scanning: %s", chartDirs)

			// Start rendering the chart
			success, errors, values, undefinedValues := renderer.RenderHelmChart(chartDir, config.ValuesFiles)

			// Protect shared variables with a mutex
			mutex.Lock()
			defer mutex.Unlock()

			// Increment the invalid chart count if the chart is invalid
			if !success {
				invalidCharts++
			}
			// Append the result to the slice of results
			results = append(results, models.Result{
				ChartPath:       chartDir,
				Success:         success,
				Errors:          errors,
				Values:          values,
				UndefinedValues: undefinedValues,
			})
		}(chartDir)
	}

	// Wait for all the goroutines to finish
	wg.Wait()
	// Stop the spinner
	s.Stop()

	// Return the slice of results and the number of invalid charts
	return results, invalidCharts
}
