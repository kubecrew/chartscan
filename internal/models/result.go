package models

type Result struct {
	ChartPath string `json:"chart_path"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}
