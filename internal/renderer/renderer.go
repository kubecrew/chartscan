package renderer

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"gopkg.in/yaml.v3"

	"github.com/Jaydee94/chartscan/internal/models"
)

var (
	nl = "\n"
	sp = " "
)

const defaultPenalty = 1e5

// TemplateParser parses a template file and extracts value references.
// It returns an array of value references and an error.
func TemplateParser(templateFile string) ([]models.ValueReference, error) {
	templateBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return nil, err
	}

	templateString := string(templateBytes)
	var valueReferences []models.ValueReference

	// Regex to capture dot notation values like .Values.service.port
	re := regexp.MustCompile(`{{\s*\.Values\.([a-zA-Z0-9_.\[\]-]+)\s*}}`)
	lines := strings.Split(templateString, "\n")

	for i, line := range lines {
		matches := re.FindAllStringSubmatchIndex(line, -1)
		for _, match := range matches {
			reference := line[match[2]:match[3]]
			if reference == "" {
				return nil, fmt.Errorf("empty value reference: %s", line[match[0]:match[1]])
			}
			valueReferences = append(valueReferences, models.ValueReference{
				Name:     reference,
				File:     templateFile,
				Line:     i + 1,
				FullText: line[match[0]:match[1]],
			})
		}
	}

	return valueReferences, nil
}

// ValuesLoader loads values from a YAML file and returns them as a map.
func ValuesLoader(valuesFile string) (map[string]interface{}, error) {
	valuesBytes, err := os.ReadFile(valuesFile)
	if err != nil {
		return nil, err
	}

	var values map[string]interface{}
	if err = yaml.Unmarshal(valuesBytes, &values); err != nil {
		return nil, err
	}

	return values, nil
}

// CheckValueReferences checks a slice of ValueReferences against a values map
// and returns a list of undefined value error strings.
func CheckValueReferences(valueReferences []models.ValueReference, values map[string]interface{}) []string {
	undefinedValues := make([]string, 0, len(valueReferences))

	for _, ref := range valueReferences {
		keys := strings.Split(ref.Name, ".")
		if !checkNestedValueExists(keys, values) {
			undefinedValues = append(undefinedValues,
				fmt.Sprintf("Undefined value: '%s' referenced in %s at line %d", ref.Name, ref.File, ref.Line),
			)
		}
	}

	return undefinedValues
}

// checkNestedValueExists recursively checks whether the nested key path
// described by keys exists within currentMap.
func checkNestedValueExists(keys []string, currentMap interface{}) bool {
	if len(keys) == 0 || currentMap == nil {
		return false
	}

	m, ok := currentMap.(map[string]interface{})
	if !ok {
		return false
	}

	if len(keys) == 1 {
		_, exists := m[keys[0]]
		return exists
	}

	next, exists := m[keys[0]].(map[string]interface{})
	if !exists {
		return false
	}

	return checkNestedValueExists(keys[1:], next)
}

// mergeMaps merges source into target, combining nested maps recursively.
// Values in source overwrite values in target at non-map keys.
func mergeMaps(target, source map[string]interface{}) {
	for key, value := range source {
		if targetMap, ok := target[key].(map[string]interface{}); ok {
			if sourceMap, ok := value.(map[string]interface{}); ok {
				mergeMaps(targetMap, sourceMap)
				continue
			}
		}
		target[key] = value
	}
}

// ScanHelmChart renders a Helm chart and checks for undefined values.
// Returns: success, errors, merged values map, and a list of undefined values.
func ScanHelmChart(chartPath string, valuesFiles []string, setValues []string) (bool, []string, map[string]interface{}, []string) {
	if chartPath == "" {
		return false, []string{"Chart path is empty"}, nil, nil
	}

	success, errors := handleDependencies(chartPath)
	if !success {
		return false, errors, nil, nil
	}

	if len(valuesFiles) > 0 {
		if missingErrors := checkValuesFilesExistence(valuesFiles); len(missingErrors) > 0 {
			return false, missingErrors, nil, nil
		}
	}

	if valuesFiles == nil {
		valuesFiles = []string{}
	}

	lintErrors := lintChart(chartPath, valuesFiles, setValues)

	valueReferences, templateErrors := parseTemplates(chartPath)
	lintErrors = append(lintErrors, templateErrors...)

	values, loadErrors := loadAndMergeValues(chartPath, valuesFiles)
	lintErrors = append(lintErrors, loadErrors...)

	if values == nil {
		values = make(map[string]interface{})
	}

	if len(setValues) > 0 {
		mergeSetValues(values, setValues)
	}

	undefinedValues := CheckValueReferences(valueReferences, values)
	allErrors := append(lintErrors, undefinedValues...)
	success = len(allErrors) == 0

	defer cleanupDependencies(chartPath)

	return success, allErrors, values, undefinedValues
}

// TemplateHelmChart renders a Helm chart using `helm template` and writes
// the output to stdout or the specified outputFile.
func TemplateHelmChart(chartPath string, valuesFiles []string, setValues []string, outputFile string) error {
	if chartPath == "" {
		return fmt.Errorf("chart path is empty")
	}

	chartPath = filepath.Clean(chartPath)
	_, releaseName := filepath.Split(chartPath)

	if releaseName == "." {
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("error getting current directory: %v", err)
		}
		_, releaseName = filepath.Split(currentDir)
	}

	releaseName = strings.TrimSpace(releaseName)
	if !isValidReleaseName(releaseName) {
		return fmt.Errorf("invalid release name: %s", releaseName)
	}

	success, errors := handleDependencies(chartPath)
	if !success {
		return fmt.Errorf("error building dependencies: %s", errors)
	}

	templateCmd := exec.Command("helm", "template", releaseName, chartPath)
	for _, vf := range valuesFiles {
		templateCmd.Args = append(templateCmd.Args, "--values", vf)
	}
	for _, sv := range setValues {
		templateCmd.Args = append(templateCmd.Args, "--set", sv)
	}

	var templateStdout, templateStderr bytes.Buffer
	templateCmd.Stdout = &templateStdout
	templateCmd.Stderr = &templateStderr

	if err := templateCmd.Run(); err != nil {
		return fmt.Errorf("error running helm template: %v\nstderr: %s", err, templateStderr.String())
	}

	if outputFile == "" {
		fmt.Println(templateStdout.String())
	} else {
		file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("error opening output file %s: %v", outputFile, err)
		}
		defer file.Close()

		if _, err := file.Write(templateStdout.Bytes()); err != nil {
			return fmt.Errorf("error writing to output file %s: %v", outputFile, err)
		}
		if _, err := file.Write([]byte("\n")); err != nil {
			return fmt.Errorf("error writing separator to output file %s: %v", outputFile, err)
		}
	}

	defer cleanupDependencies(chartPath)
	return nil
}

// isValidReleaseName returns true if name matches Helm's release name regex.
func isValidReleaseName(name string) bool {
	const releaseNamePattern = `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	return regexp.MustCompile(releaseNamePattern).MatchString(name)
}

// handleDependencies checks for and runs `helm dependency update` if the chart
// has declared dependencies. Returns success and any error messages.
func handleDependencies(chartPath string) (bool, []string) {
	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	hasDependencies, err := checkForDependencies(chartYamlPath)
	if err != nil {
		return false, []string{fmt.Sprintf("Error reading Chart.yaml: %v", err)}
	}

	if !hasDependencies {
		return true, nil
	}

	cacheDir, err := os.MkdirTemp("", "chartscan")
	if err != nil {
		return false, []string{fmt.Sprintf("Error creating temp cache dir: %v", err)}
	}
	defer os.RemoveAll(cacheDir)

	dependencyCmd := exec.Command("helm", "dependency", "update", "--repository-cache", cacheDir, chartPath)
	if err := dependencyCmd.Run(); err != nil {
		return false, []string{fmt.Sprintf("Error updating dependencies: %v", err)}
	}

	return true, nil
}

// cleanupDependencies removes the `charts/` directory and `Chart.lock` produced
// by a previous `helm dependency update` call.
func cleanupDependencies(chartPath string) {
	chartsDir := filepath.Join(chartPath, "charts")
	chartLockFile := filepath.Join(chartPath, "Chart.lock")
	defer func() {
		os.RemoveAll(chartsDir)
		os.Remove(chartLockFile)
	}()
}

// checkValuesFilesExistence returns error messages for any values file that
// does not exist on the filesystem.
func checkValuesFilesExistence(valuesFiles []string) []string {
	var errors []string
	for _, vf := range valuesFiles {
		if _, err := os.Stat(vf); os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("Values file does not exist: %s", vf))
		}
	}
	return errors
}

// lintChart runs `helm lint --strict` on the chart and returns any error messages.
func lintChart(chartPath string, valuesFiles []string, setValues []string) []string {
	lintCmd := exec.Command("helm", "lint", "--strict", chartPath)
	for _, vf := range valuesFiles {
		lintCmd.Args = append(lintCmd.Args, "--values", vf)
	}
	for _, sv := range setValues {
		lintCmd.Args = append(lintCmd.Args, "--set", sv)
	}

	var lintStdout, lintStderr bytes.Buffer
	lintCmd.Stdout = &lintStdout
	lintCmd.Stderr = &lintStderr

	if err := lintCmd.Run(); err != nil {
		return parseErrorLogs(lintStdout.String() + lintStderr.String())
	}

	return nil
}

// parseTemplates walks the chart's templates/ directory, parses YAML files,
// and returns all extracted value references together with any error messages.
func parseTemplates(chartPath string) ([]models.ValueReference, []string) {
	var valueReferences []models.ValueReference
	var errors []string

	templatesDir := filepath.Join(chartPath, "templates")
	info, err := os.Stat(templatesDir)
	if os.IsNotExist(err) {
		return valueReferences, errors
	}
	if err != nil {
		errors = append(errors, fmt.Sprintf("Error accessing templates directory: %v", err))
		return valueReferences, errors
	}
	if !info.IsDir() {
		errors = append(errors, fmt.Sprintf("Expected templates to be a directory but found a file: %s", templatesDir))
		return valueReferences, errors
	}

	err = filepath.Walk(templatesDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			errors = append(errors, fmt.Sprintf("Error accessing file %s: %v", path, walkErr))
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
			refs, err := TemplateParser(path)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Error parsing template file %s: %v", path, err))
			} else {
				valueReferences = append(valueReferences, refs...)
			}
		}
		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("Error walking templates directory: %v", err))
	}

	return valueReferences, errors
}

// loadAndMergeValues loads the chart's values.yaml and any additional values
// files, merging them into a single map. Errors are collected but do not abort.
func loadAndMergeValues(chartPath string, valuesFiles []string) (map[string]interface{}, []string) {
	values := make(map[string]interface{})
	var errors []string

	chartValuesFile := filepath.Join(chartPath, "values.yaml")

	if _, err := os.Stat(chartValuesFile); err == nil {
		if chartValues, err := ValuesLoader(chartValuesFile); err != nil {
			errors = append(errors, fmt.Sprintf("Error loading values.yaml: %v", err))
		} else if chartValues != nil {
			mergeMaps(values, chartValues)
		}
	} else if !os.IsNotExist(err) {
		errors = append(errors, fmt.Sprintf("Error checking values.yaml: %v", err))
	}

	for _, vf := range valuesFiles {
		if vf == chartValuesFile {
			continue
		}
		if additionalValues, err := ValuesLoader(vf); err != nil {
			errors = append(errors, fmt.Sprintf("Error loading additional values file %s: %v", vf, err))
		} else if additionalValues != nil {
			mergeMaps(values, additionalValues)
		}
	}

	return values, errors
}

// checkForDependencies reads Chart.yaml and returns true if the chart has a
// non-empty dependencies list.
func checkForDependencies(chartYamlPath string) (bool, error) {
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		return false, err
	}

	var chartData map[string]interface{}
	if err = yaml.Unmarshal(data, &chartData); err != nil {
		return false, err
	}

	dependencies, ok := chartData["dependencies"]
	if !ok {
		return false, nil
	}

	depsList, ok := dependencies.([]interface{})
	return ok && len(depsList) > 0, nil
}

// parseErrorLogs scans Helm command output and returns lines containing "[ERROR]".
func parseErrorLogs(output string) []string {
	var errorMessages []string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "[ERROR]") {
			errorMessages = append(errorMessages, line)
		}
	}
	return errorMessages
}

// colorSymbol returns a green or red colored symbol based on success.
func colorSymbol(s string, success bool) string {
	if success {
		return color.GreenString(s)
	}
	return color.RedString(s)
}

// colorize returns s wrapped with ANSI escape codes for the given color name.
// Supported colors: "green", "red". Unknown colors return s unchanged.
func colorize(s string, c string) string {
	switch c {
	case "green":
		return "\033[32m" + s + "\033[0m"
	case "red":
		return "\033[31m" + s + "\033[0m"
	default:
		return s
	}
}

// PrintResultsPretty prints the scan results as a formatted table, followed
// by a summary line with counts and elapsed time.
func PrintResultsPretty(results []models.Result, duration time.Duration) {
	table := tablewriter.NewTable(os.Stdout,
		tablewriter.WithHeader([]string{"Chart Name", "Success", "Details"}),
		tablewriter.WithRowAlignment(tw.AlignLeft),
	)

	var validCharts, invalidCharts int

	for _, result := range results {
		chartName, err := getChartName(result.ChartPath)
		if err != nil {
			chartName = result.ChartPath
		}

		successStr := colorSymbol("✔", result.Success)
		if result.Success {
			validCharts++
		} else {
			successStr = colorSymbol("✘", result.Success)
			invalidCharts++
		}

		errorDetails := ""
		if sanitized := sanitizeErrors(result.Errors); len(sanitized) > 0 {
			errorDetails = "• " + strings.Join(sanitized, "\n• ")
		}

		table.Append([]string{chartName, successStr, errorDetails}) //nolint:errcheck
	}

	table.Render() //nolint:errcheck

	fmt.Printf("\nSummary: %d valid charts, %d invalid charts scanned in %v\n", validCharts, invalidCharts, duration)
}

// sanitizeErrors replaces problematic characters in error messages and wraps
// long lines to a maximum of 120 characters.
func sanitizeErrors(errors []string) []string {
	var sanitized []string
	for _, err := range errors {
		// Fix: apply both replacements on sanitizedErr, not back on err
		sanitizedErr := strings.ReplaceAll(err, "|", "¦")
		sanitizedErr = strings.ReplaceAll(sanitizedErr, "\\n", "\n")
		var newLines []string
		for _, line := range strings.Split(sanitizedErr, "\n") {
			wrapped, _ := wrapString(line, 120)
			newLines = append(newLines, strings.Join(wrapped, "\n  "))
		}
		sanitized = append(sanitized, strings.Join(newLines, "\n"))
	}
	return sanitized
}

// wrapString wraps s into a paragraph of lines of at most lim rune-widths,
// with minimal raggedness. Returns the lines and the effective line limit used.
func wrapString(s string, lim int) ([]string, int) {
	words := strings.Split(strings.ReplaceAll(s, nl, sp), sp)
	var lines []string
	max := 0
	for _, v := range words {
		if w := runewidth.StringWidth(v); w > max {
			max = w
		}
	}
	if max > lim {
		lim = max
	}
	for _, line := range wrapWords(words, 1, lim, defaultPenalty) {
		lines = append(lines, strings.Join(line, sp))
	}
	return lines, lim
}

// wrapWords splits words into lines with minimal raggedness using a
// dynamic-programming approach. spc is the space between words, lim is the
// target line width in runes, and pen is the extra penalty for over-long lines.
func wrapWords(words []string, spc, lim, pen int) [][]string {
	n := len(words)

	length := make([][]int, n)
	for i := 0; i < n; i++ {
		length[i] = make([]int, n)
		length[i][i] = runewidth.StringWidth(words[i])
		for j := i + 1; j < n; j++ {
			length[i][j] = length[i][j-1] + spc + runewidth.StringWidth(words[j])
		}
	}

	nbrk := make([]int, n)
	cost := make([]int, n)
	for i := range cost {
		cost[i] = math.MaxInt32
	}

	for i := n - 1; i >= 0; i-- {
		if length[i][n-1] <= lim {
			cost[i] = 0
			nbrk[i] = n
		} else {
			for j := i + 1; j < n; j++ {
				d := lim - length[i][j-1]
				c := d*d + cost[j]
				if length[i][j-1] > lim {
					c += pen
				}
				if c < cost[i] {
					cost[i] = c
					nbrk[i] = j
				}
			}
		}
	}

	var lines [][]string
	for i := 0; i < n; {
		lines = append(lines, words[i:nbrk[i]])
		i = nbrk[i]
	}
	return lines
}

// getChartName reads Chart.yaml from the given chart directory and returns
// the value of the "name" field.
func getChartName(chartPath string) (string, error) {
	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	data, err := os.ReadFile(chartYamlPath)
	if err != nil {
		return "", fmt.Errorf("error reading Chart.yaml: %v", err)
	}

	var chartData map[string]interface{}
	if err = yaml.Unmarshal(data, &chartData); err != nil {
		return "", fmt.Errorf("error parsing Chart.yaml: %v", err)
	}

	name, ok := chartData["name"].(string)
	if !ok {
		return "", fmt.Errorf("missing or invalid name in Chart.yaml")
	}

	return name, nil
}

// mergeSetValues parses "key=value" strings and sets the resulting values in
// the values map, creating nested maps for dot-separated key paths.
// Boolean and integer values are parsed automatically.
func mergeSetValues(values map[string]interface{}, setValues []string) {
	for _, sv := range setValues {
		parts := strings.SplitN(sv, "=", 2)
		if len(parts) != 2 {
			continue
		}

		keyPath, valueStr := parts[0], parts[1]

		var value interface{} = valueStr
		if b, err := strconv.ParseBool(valueStr); err == nil {
			value = b
		} else if i, err := strconv.Atoi(valueStr); err == nil {
			value = i
		} else if f, err := strconv.ParseFloat(valueStr, 64); err == nil {
			value = f
		}

		keys := strings.Split(keyPath, ".")
		currentMap := values
		for i, key := range keys {
			if i == len(keys)-1 {
				currentMap[key] = value
				break
			}
			if _, ok := currentMap[key].(map[string]interface{}); !ok {
				currentMap[key] = make(map[string]interface{})
			}
			currentMap = currentMap[key].(map[string]interface{})
		}
	}
}
