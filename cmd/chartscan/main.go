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
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var version = "dev"

func main() {
	var configFile string
	var listEnvironments bool

	rootCmd := &cobra.Command{
		Use:   "chartscan",
		Short: "ChartScan is a tool to scan Helm charts",
		PreRun: func(cmd *cobra.Command, args []string) {
			if configFile == "" {
				var err error
				configFile, err = loadConfigFileFromGitRepo()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error checking Git repo: %v\n", err)
					os.Exit(1)
				}
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if listEnvironments {
				if err := listConfiguredEnvironments(configFile); err != nil {
					fmt.Fprintf(os.Stderr, "Error listing environments: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			}
			if len(args) == 0 {
				cmd.Help() //nolint:errcheck
				os.Exit(0)
			}
		},
	}

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.PersistentFlags().BoolVarP(&listEnvironments, "list-environments", "l", false, "List all configured environments if a chartscan.yaml is found or explicitly passed")

	rootCmd.AddCommand(buildScanCmd())
	rootCmd.AddCommand(buildTemplateCmd())
	rootCmd.AddCommand(buildVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// buildScanCmd constructs and returns the `scan` subcommand.
func buildScanCmd() *cobra.Command {
	var (
		configFile  string
		valuesFiles []string
		format      string
		environment string
		failOnError bool
		setValues   []string
	)

	cmd := &cobra.Command{
		Use:   "scan [chart-path]",
		Short: "Scan Helm charts for potential issues",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if configFile == "" {
				var err error
				configFile, err = loadConfigFileFromGitRepo()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error checking Git repo: %v\n", err)
					os.Exit(1)
				}
			}

			config, err := loadConfig(configFile, valuesFiles, format, args, environment)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}

			startTime := time.Now()
			var chartDirs []string
			for _, chartPath := range args {
				dirs, err := finder.FindHelmChartDirs(chartPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error finding Helm charts in %s: %v\n", chartPath, err)
					os.Exit(1)
				}
				chartDirs = append(chartDirs, dirs...)
			}

			results, invalidCharts := processCharts(chartDirs, *config, setValues)
			duration := time.Since(startTime)

			var output []byte
			switch config.Format {
			case "pretty":
				renderer.PrintResultsPretty(results, duration)
			case "json":
				output, err = json.MarshalIndent(results, "", "  ")
			case "yaml":
				output, err = yaml.Marshal(results)
			case "junit":
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
				fmt.Println(string(output))
			}

			if failOnError && invalidCharts > 0 {
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	cmd.Flags().StringSliceVarP(&valuesFiles, "values", "f", []string{}, "Specify values files for rendering (optional)")
	cmd.Flags().StringVarP(&format, "output-format", "o", "pretty", "Output format (pretty, json, yaml, junit)")
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "(Optional) Specify the environment to use (e.g., test, staging, production).")
	cmd.Flags().BoolVar(&failOnError, "fail-on-error", false, "Exit with error code 1 if there are invalid charts")
	cmd.Flags().StringSliceVar(&setValues, "set", []string{}, "Set values on the command line (key1=val1,key2=val2)")

	return cmd
}

// buildTemplateCmd constructs and returns the `template` subcommand.
func buildTemplateCmd() *cobra.Command {
	var (
		configFile  string
		valuesFiles []string
		outputFile  string
		environment string
		setValues   []string
	)

	cmd := &cobra.Command{
		Use:   "template [chart-path]...",
		Short: "Render Helm charts using helm template",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if configFile == "" {
				var err error
				configFile, err = loadConfigFileFromGitRepo()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error checking Git repo: %v\n", err)
					os.Exit(1)
				}
			}

			config, err := loadConfig(configFile, valuesFiles, "", args, environment)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}

			s := spinner.New(spinner.CharSets[4], 100*time.Millisecond)
			s.Start()
			defer s.Stop()

			for _, chartPath := range args {
				s.Suffix = fmt.Sprintf(" Templating: %s", chartPath)
				if err := renderer.TemplateHelmChart(chartPath, config.ValuesFiles, setValues, outputFile); err != nil {
					fmt.Fprintf(os.Stderr, "Error rendering chart %s: %v\n", chartPath, err)
					s.Stop()
					os.Exit(1)
				}
			}
		},
	}

	cmd.Flags().StringSliceVarP(&valuesFiles, "values", "f", nil, "Specify values files for rendering")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file to write the rendered chart (optional)")
	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "(Optional) Specify the environment to use.")
	cmd.Flags().StringSliceVar(&setValues, "set", []string{}, "Set values on the command line (key1=val1,key2=val2)")

	return cmd
}

// buildVersionCmd constructs and returns the `version` subcommand.
func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of ChartScan",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("ChartScan version %s\n", version)
		},
	}
}

// checkIfInGitRepo returns true if the current directory is inside a Git
// repository, along with the repository root path.
func checkIfInGitRepo() (bool, string, error) {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	output, err := cmd.Output()
	if err != nil {
		return false, "", err
	}

	if strings.TrimSpace(string(output)) != "true" {
		return false, "", nil
	}

	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	rootDirOutput, err := cmd.Output()
	if err != nil {
		return false, "", err
	}

	return true, strings.TrimSpace(string(rootDirOutput)), nil
}

// findConfigFileInGitRepo returns the path to chartscan.yaml in the repo root,
// or an empty string if the file does not exist.
func findConfigFileInGitRepo(rootDir string) string {
	configFilePath := filepath.Join(rootDir, "chartscan.yaml")
	if _, err := os.Stat(configFilePath); err == nil {
		return configFilePath
	}
	return ""
}

// loadConfigFileFromGitRepo checks whether we are inside a Git repository and
// returns the path to chartscan.yaml in the repo root, if present.
func loadConfigFileFromGitRepo() (string, error) {
	isInRepo, rootDir, err := checkIfInGitRepo()
	if err != nil {
		return "", err
	}

	if isInRepo {
		if configFile := findConfigFileInGitRepo(rootDir); configFile != "" {
			fmt.Printf("Using config file from project root: %s\n", configFile)
			return configFile, nil
		}
	}

	return "", nil
}

// listConfiguredEnvironments prints all environments defined in the config file
// as a formatted table.
func listConfiguredEnvironments(configFile string) error {
	config, err := loadConfigFromFile(configFile)
	if err != nil {
		return err
	}

	if config.Environments == nil {
		fmt.Println("No environments configured.")
		return nil
	}

	table := tablewriter.NewTable(os.Stdout,
		tablewriter.WithHeader([]string{"Environment", "Values Files"}),
		tablewriter.WithRowAlignment(tw.AlignLeft),
	)

	for env, envConfig := range config.Environments {
		valuesFiles := ""
		if len(envConfig.ValuesFiles) > 0 {
			valuesFiles = "• " + strings.Join(envConfig.ValuesFiles, "\n• ")
		}
		table.Append([]string{env, valuesFiles}) //nolint:errcheck
	}

	table.Render() //nolint:errcheck
	return nil
}

// loadConfigFromFile reads and unmarshals the YAML configuration file.
// If configFile is empty, it attempts to discover it from the Git repo root.
func loadConfigFromFile(configFile string) (*models.Config, error) {
	config := &models.Config{}

	if configFile == "" {
		var err error
		configFile, err = loadConfigFileFromGitRepo()
		if err != nil {
			return config, err
		}
	}

	if configFile == "" {
		return config, nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

// printJUnitTestReport generates a JUnit-compatible XML test report from results
// and prints it to stdout.
func printJUnitTestReport(results []models.Result) error {
	var testCases []models.TestCase
	failures := 0

	for _, result := range results {
		testCase := models.TestCase{
			Name:      result.ChartPath,
			ClassName: "ChartScan",
			Time:      "0",
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

// loadConfig builds a Config from the config file and CLI overrides.
func loadConfig(configFile string, valuesFiles []string, format string, args []string, environment string) (*models.Config, error) {
	config := &models.Config{}

	if configFile != "" {
		configDir := filepath.Dir(configFile)
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, err
		}

		config.ChartPath, err = resolveRelativePath(configDir, config.ChartPath)
		if err != nil {
			return nil, fmt.Errorf("error resolving chartPath: %v", err)
		}
	}

	if environment != "" {
		envConfig, exists := config.Environments[environment]
		if !exists {
			return nil, fmt.Errorf("environment %s not found in chartscan.yaml", environment)
		}
		if len(envConfig.ValuesFiles) > 0 {
			config.ValuesFiles = envConfig.ValuesFiles
		} else {
			config.ValuesFiles = nil
		}
	}

	if len(valuesFiles) > 0 {
		config.ValuesFiles = valuesFiles
	}
	if format != "" {
		config.Format = format
	}

	if configFile != "" {
		configDir := filepath.Dir(configFile)
		for i, vf := range config.ValuesFiles {
			resolved, err := resolveRelativePath(configDir, vf)
			if err != nil {
				return config, fmt.Errorf("error resolving valuesFile %s: %v", vf, err)
			}
			config.ValuesFiles[i] = resolved
		}
	}

	return config, nil
}

// resolveRelativePath joins relativePath with baseDir and returns the absolute path.
func resolveRelativePath(baseDir, relativePath string) (string, error) {
	return filepath.Abs(filepath.Join(baseDir, relativePath))
}

// processCharts scans chart directories concurrently and returns results with
// the total count of invalid charts.
func processCharts(chartDirs []string, config models.Config, setValues []string) ([]models.Result, int) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	results := make([]models.Result, 0, len(chartDirs))
	invalidCharts := 0

	s := spinner.New(spinner.CharSets[4], 100*time.Millisecond)
	s.Start()
	defer s.Stop()

	wg.Add(len(chartDirs))
	for _, chartDir := range chartDirs {
		go func(chartDir string) {
			defer wg.Done()

			// Fix: use chartDir (individual path) not chartDirs (entire slice)
			s.Suffix = fmt.Sprintf(" Scanning: %s", chartDir)

			success, errors, values, undefinedValues := renderer.ScanHelmChart(chartDir, config.ValuesFiles, setValues)

			mu.Lock()
			defer mu.Unlock()

			if !success && len(errors) > 0 {
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
	return results, invalidCharts
}
