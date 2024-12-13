# ChartScan

**ChartScan** is a CLI tool for scanning and analyzing Helm charts. It provides insights into Helm chart configurations, values, and rendering issues, allowing developers to efficiently debug and validate Helm charts before deployment.

---

## Features

- Scans directories for Helm charts.
- Supports multiple values files for rendering charts.
- Configurable output formats: **pretty**, **JSON**, or **YAML**.
- Supports configuration through YAML-based config files.
- Includes a loading indicator during processing.
- Verifies that the Helm CLI is installed before execution.

---

## Installation

1. Clone the repository:

   ```bash
   git clone https://github.com/Jaydee94/chartscan.git
   cd chartscan
   ```

2. Build the binary:

   ```bash
   go build -o chartscan
   ```

3. (Optional) Move the binary to your PATH:

   ```bash
   mv chartscan /usr/local/bin
   ```

---

## Prerequisites

Ensure the following dependencies are installed:

- **Go**: [Install Go](https://golang.org/dl/)
- **Helm**: [Install Helm](https://helm.sh/docs/intro/install/)

---

## Usage

### Basic Command

```bash
chartscan [chart-path]
```

### Options

- `-f, --values`: Specify values files to use for rendering.
- `-o, --format`: Set the output format (pretty, json, yaml). Default is `pretty`.
- `-c, --config`: Provide a configuration file (YAML format) to override CLI flags.

### Example

#### Scan a Chart Directory with Values Files:
```bash
chartscan ./charts -f values.yaml -o json
```

#### Use a Config File:
```bash
chartscan -c config.yaml
```

#### Example Config File:
```yaml
chartPath: ./charts
valuesFiles:
  - values.yaml
format: yaml
```

---

## Output Formats

- **Pretty**: Human-readable formatted output.
- **JSON**: Machine-readable JSON format.
- **YAML**: YAML-encoded output for further processing.

---

## Development

### Running Locally

1. Clone the repository.
2. Install dependencies:

   ```bash
   go mod tidy
   ```

3. Run the tool:

   ```bash
   go run main.go [options]
   ```

### Testing

Run the test suite:

```bash
go test ./...
```

---

## Contribution

Contributions are welcome! Please follow these steps:

1. Fork the repository.
2. Create a feature branch (`git checkout -b feature-name`).
3. Commit your changes (`git commit -m "Add feature"`).
4. Push to the branch (`git push origin feature-name`).
5. Open a pull request.

---

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

## Acknowledgments

- Built with [Cobra](https://github.com/spf13/cobra) for CLI management.
- Utilizes [briandowns/spinner](https://github.com/briandowns/spinner) for loading indicators.
- Inspired by the need for streamlined Helm chart debugging.
