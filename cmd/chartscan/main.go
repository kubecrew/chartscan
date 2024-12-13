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
type Config struct {
	ChartPath   string   `yaml:"chartPath"`   // Base path to search for Helm charts
	ValuesFiles []string `yaml:"valuesFiles"` // List of values files to use during rendering
	Format      string   `yaml:"format"`      // Output format: pretty, json, yaml, gitlab
}

// TestSuite represents a JUnit-style test suite for GitLab reports
type TestSuite struct {
	XMLName    xml.Name   `xml:"testsuite"`
	Name       string     `xml:"name,attr"`
	Tests      int        `xml:"tests,attr"`
	Failures   int        `xml:"failures,attr"`
	Time       string     `xml:"time,attr"`
	TestCases  []TestCase `xml:"testcase"`
	Properties []Property `xml:"properties>property,omitempty"`
}

// TestCase represents a single test case in a JUnit-style test report
type TestCase struct {
	Name      string     `xml:"name,attr"`
	ClassName string     `xml:"classname,attr"`
	Time      string     `xml:"time,attr"`
	Failure   *Failure   `xml:"failure,omitempty"`
	SystemOut *SystemOut `xml:"system-out,omitempty"`
}

// Failure represents a failure in a test case
type Failure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// SystemOut captures stdout for a test case
type SystemOut struct {
	Content string `xml:",chardata"`
}

// Property represents a property in the JUnit test suite
type Property struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

func main() {
	var configFile string
	var valuesFiles []string
	var format string

	rootCmd := &cobra.Command{
		Use:   "chartscan [chart-path]",
		Short: "ChartScan is a tool to scan Helm charts",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			config, err := loadConfig(configFile, valuesFiles, format, args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}

			chartDirs, err := finder.FindHelmChartDirs(config.ChartPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error finding Helm charts: %v\n", err)
				os.Exit(1)
			}

			results, invalidCharts := processCharts(chartDirs, config)

			switch config.Format {
			case "pretty":
				renderer.PrintResultsPretty(results)
			case "json":
				output, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error marshaling results to JSON: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(string(output))
			case "yaml":
				output, err := yaml.Marshal(results)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error marshaling results to YAML: %v\n", err)
					os.Exit(1)
				}
				fmt.Println(string(output))
			case "gitlab":
				err := printGitLabUnitTestReport(results)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error generating GitLab unit test report: %v\n", err)
					os.Exit(1)
				}
			default:
				fmt.Fprintf(os.Stderr, "Unknown output format: %s\n", config.Format)
				os.Exit(1)
			}

			if invalidCharts > 0 {
				os.Exit(1)
			}
		},
	}

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.Flags().StringSliceVarP(&valuesFiles, "values", "f", nil, "Specify values files for rendering")
	rootCmd.Flags().StringVarP(&format, "format", "o", "pretty", "Output format (pretty, json, yaml, gitlab)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// printGitLabUnitTestReport generates a GitLab-compatible unit test report
func printGitLabUnitTestReport(results []models.Result) error {
	suite := TestSuite{
		Name:  "Helm Chart Scan",
		Tests: len(results),
	}

	for _, result := range results {
		testCase := TestCase{
			Name:      result.ChartPath,
			ClassName: "ChartScan",
			Time:      "0", // Dummy value for now; can measure rendering time if required
		}

		if !result.Success {
			testCase.Failure = &Failure{
				Message: "Chart rendering failed",
				Type:    "RenderingError",
				Content: fmt.Sprintf("Errors: %v\nUndefined Values: %v", result.Errors, result.UndefinedValues),
			}
		} else {
			testCase.SystemOut = &SystemOut{
				Content: fmt.Sprintf("Chart %v rendered successfully", result.ChartPath),
			}
		}

		if !result.Success {
			suite.Failures++
		}
		suite.TestCases = append(suite.TestCases, testCase)
	}

	output, err := xml.MarshalIndent(suite, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(output))
	return nil
}

// loadConfig dynamically loads the configuration from a file and/or CLI arguments
func loadConfig(configFile string, valuesFiles []string, format string, args []string) (Config, error) {
	var config Config

	// Load from configuration file if specified
	if configFile != "" {
		file, err := os.Open(configFile)
		if err != nil {
			return config, fmt.Errorf("error opening config file: %v", err)
		}
		defer file.Close()

		decoder := yaml.NewDecoder(file)
		if err := decoder.Decode(&config); err != nil {
			return config, fmt.Errorf("error decoding config file: %v", err)
		}
	}

	// Override with CLI arguments and defaults
	if len(args) > 0 {
		config.ChartPath = args[0]
	} else if config.ChartPath == "" {
		config.ChartPath = "./charts" // Default path
	}

	if len(valuesFiles) > 0 {
		config.ValuesFiles = valuesFiles
	}

	if format != "" {
		config.Format = format
	} else if config.Format == "" {
		config.Format = "pretty" // Default format
	}

	return config, nil
}

// processCharts scans and processes all chart directories concurrently
func processCharts(chartDirs []string, config Config) ([]models.Result, int) {
	var wg sync.WaitGroup
	var mutex sync.Mutex

	results := []models.Result{}
	invalidCharts := 0

	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Start()
	defer s.Stop()

	wg.Add(len(chartDirs))
	for _, chartDir := range chartDirs {
		go func(chartDir string) {
			defer wg.Done()

			s.Suffix = fmt.Sprintf(" Scanning charts: %s", chartDirs)
			success, errors, values, undefinedValues := renderer.RenderHelmChart(chartDir, config.ValuesFiles)

			// Protect shared variables with a mutex
			mutex.Lock()
			defer mutex.Unlock()

			if !success {
				invalidCharts++
			}
			results = append(results, models.Result{
				ChartPath:       chartDir,
				Success:         success,
				Errors:          errors,
				Values:          values,
				UndefinedValues: undefinedValues,
			})
		}(chartDir)
	}

	wg.Wait()
	s.Stop()
	return results, invalidCharts
}
