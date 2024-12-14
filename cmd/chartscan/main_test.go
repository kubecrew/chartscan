package main

import (
	"os"
	"testing"

	"github.com/Jaydee94/chartscan/internal/models"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestLoadConfigFromFile(t *testing.T) {
	// Create a temporary file with valid YAML
	tmpFile, err := os.CreateTemp("", "chartscan-config")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	yamlConfig := models.Config{
		ChartPath:   "./charts",
		ValuesFiles: []string{"values.yaml"},
		Format:      "json",
	}
	yamlBytes, err := yaml.Marshal(yamlConfig)
	assert.NoError(t, err)

	_, err = tmpFile.Write(yamlBytes)
	assert.NoError(t, err)

	// Load configuration from file
	config, err := loadConfig(tmpFile.Name(), nil, "", nil)
	assert.NoError(t, err)

	assert.Equal(t, yamlConfig.ChartPath, config.ChartPath)
	assert.Equal(t, yamlConfig.ValuesFiles, config.ValuesFiles)
	assert.Equal(t, yamlConfig.Format, config.Format)
}

func TestLoadConfigFromFileInvalidYAML(t *testing.T) {
	// Create a temporary file with invalid YAML
	tmpFile, err := os.CreateTemp("", "chartscan-config")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(" invalid yaml ")
	assert.NoError(t, err)

	// Load configuration from file
	_, err = loadConfig(tmpFile.Name(), nil, "", nil)
	assert.Error(t, err)
}

func TestLoadConfigFromFileNonExistent(t *testing.T) {
	// Load configuration from non-existent file
	_, err := loadConfig("non-existent-file.yaml", nil, "", nil)
	assert.Error(t, err)
}

func TestLoadConfigWithCLIArgs(t *testing.T) {
	// Load configuration with CLI arguments
	config, err := loadConfig("", nil, "json", []string{"./charts"})
	assert.NoError(t, err)

	assert.Equal(t, "./charts", config.ChartPath)
	assert.Equal(t, "json", config.Format)
}

func TestLoadConfigWithDefaultValues(t *testing.T) {
	// Load configuration with default values
	config, err := loadConfig("", nil, "", nil)
	assert.NoError(t, err)

	assert.Equal(t, "./charts", config.ChartPath)
	assert.Equal(t, "pretty", config.Format)
}

func TestLoadConfigWithMultipleValuesFiles(t *testing.T) {
	// Load configuration with multiple values files
	config, err := loadConfig("", []string{"values1.yaml", "values2.yaml"}, "", nil)
	assert.NoError(t, err)

	assert.Equal(t, []string{"values1.yaml", "values2.yaml"}, config.ValuesFiles)
}

func TestLoadConfigWithMultipleCLIArgs(t *testing.T) {
	// Load configuration with multiple CLI arguments
	_, err := loadConfig("", nil, "", []string{"./charts", "extra-arg"})
	assert.NoError(t, err)
}
