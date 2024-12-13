package models

type Result struct {
	ChartPath       string                 `json:"ChartPath"`
	Success         bool                   `json:"Success"`
	Errors          []string               `json:"Errors,omitempty"`
	UndefinedValues []string               `json:"UndefinedValues,omitempty"`
	Values          map[string]interface{} `json:"Values,omitempty"`
}

type ValueReference struct {
	Name     string
	File     string
	Line     int
	FullText string
}
