package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	// outputFile for specifying the output file for the rendered chart
	var outputFile string

	// Root command
	rootCmd := &cobra.Command{
		Use:   "chartscan",
		Short: "ChartScan is a tool to scan Helm charts",
	}

	// Scan subcommand
	scanCmd := &cobra.Command{
		Use:   "scan [chart-path]",
		Short: "Scan Helm charts for potential issues",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Automatically load the config file from the git repo if possible
			configFile, err := loadConfigFileFromGitRepo()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error checking Git repo: %v\n", err)
				os.Exit(1)
			}
			// Load the configuration from the configuration file and/or CLI arguments
			config, err := loadConfig(configFile, valuesFiles, format, args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}

			startTime := time.Now()

			var chartDirs []string
			// Iterate over all chart paths passed in args
			for _, chartPath := range args {
				// Find the Helm charts to scan in each path
				dirs, err := finder.FindHelmChartDirs(chartPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error finding Helm charts in %s: %v\n", chartPath, err)
					os.Exit(1)
				}
				chartDirs = append(chartDirs, dirs...) // Combine the directories found
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

	// template subcommand
	templateCmd := &cobra.Command{
		Use:   "template [chart-path]...",
		Short: "Render Helm charts using helm template",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Automatically load the config file from the git repo if possible
			configFile, err := loadConfigFileFromGitRepo()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error checking Git repo: %v\n", err)
				os.Exit(1)
			}
			// Load the configuration from the configuration file and/or CLI arguments
			config, err := loadConfig(configFile, valuesFiles, format, args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}

			// Create a spinner to indicate progress
			s := spinner.New(spinner.CharSets[4], 100*time.Millisecond)
			s.Start()
			defer s.Stop()

			chartPaths := args
			// Call TemplateHelmChart for each chart provided
			for _, chartPath := range chartPaths {
				// Update the spinner with the chart being rendered
				s.Suffix = fmt.Sprintf(" Templating: %s", chartPaths)

				err := renderer.TemplateHelmChart(chartPath, config.ValuesFiles, outputFile)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error rendering chart %s: %v\n", chartPath, err)
					s.Stop() // Stop the spinner on error
					os.Exit(1)
				}
			}

			// Stop the spinner after all charts are processed
			s.Stop()
		},
	}

	// Add flags to the template subcommand
	templateCmd.Flags().StringSliceVarP(&valuesFiles, "values", "f", nil, "Specify values files for rendering")
	templateCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file to write the rendered chart (optional)")
	templateCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file") // Ensure this flag is added here

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
	rootCmd.AddCommand(templateCmd)
	rootCmd.AddCommand(versionCmd)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// checkIfInGitRepo checks if the current working directory is inside a Git repository
func checkIfInGitRepo() (bool, string, error) {
	// Run `git rev-parse --is-inside-work-tree` to check if we're inside a git repo
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	output, err := cmd.Output()
	if err != nil {
		return false, "", err
	}
	// If the output is "true", we're in a git repository
	if strings.TrimSpace(string(output)) == "true" {
		// Run `git rev-parse --show-toplevel` to get the root directory of the git repo
		cmd = exec.Command("git", "rev-parse", "--show-toplevel")
		rootDirOutput, err := cmd.Output()
		if err != nil {
			return false, "", err
		}
		rootDir := strings.TrimSpace(string(rootDirOutput))
		return true, rootDir, nil
	}
	return false, "", nil
}

// findConfigFileInGitRepo checks if the `chartscan.yaml` file exists in the root of the Git repo
func findConfigFileInGitRepo(rootDir string) string {
	// Look for the chartscan.yaml file in the root of the repo
	configFilePath := filepath.Join(rootDir, "chartscan.yaml")
	if _, err := os.Stat(configFilePath); err == nil {
		// If the file exists, return its path
		return configFilePath
	}
	return ""
}

// loadConfigFileFromGitRepo checks if we are in a Git repository and if
// the chartscan.yaml file exists in the root of the Git repo
func loadConfigFileFromGitRepo() (string, error) {
	// Check if we are in a Git repository
	isInRepo, rootDir, err := checkIfInGitRepo()
	if err != nil {
		return "", err
	}

	if isInRepo {
		// If we're inside a Git repo, look for the chartscan.yaml in the repo root
		configFile := findConfigFileInGitRepo(rootDir)
		if configFile != "" {
			// Notify that the config file was found
			fmt.Printf("Using config file from project root: %s\n", configFile)
			return configFile, nil
		}
	}

	return "", nil
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
func loadConfig(configFile string, valuesFiles []string, format string, args []string) (models.Config, error) {
	config := models.Config{}

	// Load from configuration file if specified
	if configFile != "" {
		// Get the directory of the config file
		configDir := filepath.Dir(configFile)

		data, err := os.ReadFile(configFile)
		if err != nil {
			return config, fmt.Errorf("error reading config file: %v", err)
		}

		// Unmarshal the configuration from the file
		if err := yaml.Unmarshal(data, &config); err != nil {
			return config, fmt.Errorf("error decoding config file: %v", err)
		}

		// Resolve relative paths for chart path and values files
		config.ChartPath, err = resolveRelativePath(configDir, config.ChartPath)
		if err != nil {
			return config, fmt.Errorf("error resolving chartPath: %v", err)
		}

		// Resolve relative values files
		for i, valuesFile := range config.ValuesFiles {
			config.ValuesFiles[i], err = resolveRelativePath(configDir, valuesFile)
			if err != nil {
				return config, fmt.Errorf("error resolving valuesFile %s: %v", valuesFile, err)
			}
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

// resolveRelativePath resolves a relative path based on the given base directory
func resolveRelativePath(baseDir, relativePath string) (string, error) {
	// Resolve relative path to absolute path based on the baseDir
	// This makes sure the paths are valid regardless of the current working directory
	absolutePath := filepath.Join(baseDir, relativePath)
	// Normalize the path to avoid issues with .. or redundant slashes
	return filepath.Abs(absolutePath)
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
	s := spinner.New(spinner.CharSets[4], 100*time.Millisecond)
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
			success, errors, values, undefinedValues := renderer.ScanHelmChart(chartDir, config.ValuesFiles)

			// Protect shared variables with a mutex
			mutex.Lock()
			defer mutex.Unlock()

			// Increment the invalid chart count if the chart is invalid
			if !success {
				invalidCharts++
			}

			// Append the result to the results slice
			results = append(results, models.Result{
				ChartPath:       chartDir, // Corrected from "Name" to "ChartPath"
				Success:         success,
				Errors:          errors,
				Values:          values,
				UndefinedValues: undefinedValues,
			})
		}(chartDir)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Return the slice of results and the number of invalid charts
	return results, invalidCharts
}
