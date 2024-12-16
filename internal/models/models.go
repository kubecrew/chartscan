package models

import "encoding/xml"

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

type EnvironmentConfig struct {
	ValuesFiles []string `yaml:"valuesFiles"`
}

type Config struct {
	ChartPath    string                       `yaml:"chartPath"`
	ValuesFiles  []string                     `yaml:"valuesFiles"`
	Format       string                       `yaml:"format"`
	Environments map[string]EnvironmentConfig `yaml:"environments"`
}

// TestSuite represents a JUnit-style test suite for test reports
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
