package models

type Result struct {
	ChartPath string   `json:"chartPath" yaml:"chartPath"`
	Success   bool     `json:"success" yaml:"success"`
	Errors    []string `json:"errors" yaml:"errors"`
}
