package renderer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Jaydee94/chartscan/internal/models"
)

func TestValuesLoader(t *testing.T) {
	tempDir := t.TempDir()
	valuesFile := filepath.Join(tempDir, "values.yaml")
	yamlContent := []byte(`
foo:
  bar: 123
  baz: true
`)
	if err := os.WriteFile(valuesFile, yamlContent, 0644); err != nil {
		t.Fatalf("Failed to create test values file: %v", err)
	}

	values, err := ValuesLoader(valuesFile)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if values == nil {
		t.Fatal("Expected values map, got nil")
	}

	fooMap, ok := values["foo"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected foo to be map, got %T", values["foo"])
	}

	if fooMap["bar"] != 123 && fooMap["bar"] != 123.0 { // yaml numbers decode to float64 or int contextually
		if intVal, ok := fooMap["bar"].(int); !ok || intVal != 123 {
			t.Errorf("Expected foo.bar to be 123, got %v", fooMap["bar"])
		}
	}
}

func TestTemplateParser(t *testing.T) {
	tempDir := t.TempDir()
	templateFile := filepath.Join(tempDir, "deployment.yaml")
	templateContent := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Values.appName }}
spec:
  template:
    spec:
      containers:
        - name: {{ .Values.app.containerName }}
          image: "nginx:latest"
`)
	if err := os.WriteFile(templateFile, templateContent, 0644); err != nil {
		t.Fatalf("Failed to create test template file: %v", err)
	}

	refs, err := TemplateParser(templateFile)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("Expected 2 value references, got %d", len(refs))
	}

	if refs[0].Name != "appName" {
		t.Errorf("Expected 'appName', got '%s'", refs[0].Name)
	}
	if refs[1].Name != "app.containerName" {
		t.Errorf("Expected 'app.containerName', got '%s'", refs[1].Name)
	}
}

func TestCheckValueReferences(t *testing.T) {
	refs := []models.ValueReference{
		{Name: "app.name", File: "test.yaml", Line: 1, FullText: "{{ .Values.app.name }}"},
		{Name: "app.missing", File: "test.yaml", Line: 2, FullText: "{{ .Values.app.missing }}"},
		{Name: "global.db.port", File: "test.yaml", Line: 3, FullText: "{{ .Values.global.db.port }}"},
	}

	values := map[string]interface{}{
		"app": map[string]interface{}{
			"name": "myapp",
		},
		// global.db is missing entirely
	}

	undefined := CheckValueReferences(refs, values)

	if len(undefined) != 2 {
		t.Fatalf("Expected 2 undefined references, got %d", len(undefined))
	}

	// Should report app.missing and global.db.port as missing
}

func TestSanitizeErrors(t *testing.T) {
	errors := []string{
		"Error: string with | pipes | and \n newlines",
	}

	sanitized := sanitizeErrors(errors)

	if len(sanitized) != 1 {
		t.Fatalf("Expected 1 sanitized error, got %d", len(sanitized))
	}

	if sanitized[0] == errors[0] { // simple assert it modified it
		t.Fatalf("Expected sanitized string to be different")
	}
}

func TestMergeSetValues(t *testing.T) {
	values := map[string]interface{}{
		"existing": "value",
	}

	setValues := []string{
		"newStr=strvalue",
		"newInt=123",
		"newFloat=1.23",
		"newBool=true",
		"nested.key=val",
	}

	mergeSetValues(values, setValues)

	if values["existing"] != "value" {
		t.Errorf("Expected existing=value, got %v", values["existing"])
	}
	if values["newStr"] != "strvalue" {
		t.Errorf("Expected newStr=strvalue, got %v", values["newStr"])
	}
	if values["newInt"] != 123 {
		t.Errorf("Expected newInt=123, got %v", values["newInt"])
	}
	if values["newFloat"] != 1.23 {
		t.Errorf("Expected newFloat=1.23, got %v", values["newFloat"])
	}
	if values["newBool"] != true {
		t.Errorf("Expected newBool=true, got %v", values["newBool"])
	}

	nested, ok := values["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected nested to be map[string]interface{}, got %T", values["nested"])
	}

	if nested["key"] != "val" {
		t.Errorf("Expected nested.key=val, got %v", nested["key"])
	}
}
