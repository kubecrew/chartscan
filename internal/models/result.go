package models

type Result struct {
	ChartPath       string                 `json:"ChartPath"`
	Success         bool                   `json:"Success"`
	Errors          []string               `json:"Errors,omitempty"`
	UndefinedValues []string               `json:"UndefinedValues,omitempty"`
	Values          map[string]interface{} `json:"Values,omitempty"`
}

type ValueReference struct {
	Name     string `json:"Name"`
	File     string `json:"File"`
	Line     int    `json:"Line"`
	FullText string `json:"FullText"`
}
