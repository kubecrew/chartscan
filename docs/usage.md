# Usage

This page documents every ChartScan command, every flag, and a recipe for each common workflow. For configuration file syntax see [configuration.md](configuration.md).

## Commands at a glance

| Command    | Purpose                                                    |
|------------|------------------------------------------------------------|
| `scan`     | Discover Helm charts, render them, report errors and undefined values. |
| `template` | Render one or more charts with `helm template`.            |
| `version`  | Print the ChartScan version.                               |

## Global flags

These flags work on the root command and on every subcommand.

| Flag                       | Description                                                                                  |
|----------------------------|----------------------------------------------------------------------------------------------|
| `-c, --config <path>`      | Path to a `chartscan.yaml` configuration file.                                               |
| `-l, --list-environments`  | List every environment defined in the resolved config file and exit. Works with `-c` or with auto-discovery in a Git repo. |
| `-h, --help`               | Show help for the current command.                                                           |

---

## `scan`

Discover Helm charts under one or more paths, render each one, and report the result.

**Synopsis**

```text
chartscan scan [chart-path]... [flags]
```

At least one chart path is required. Each path may be a single chart directory or a parent directory that contains many charts — ChartScan recurses and treats every directory that contains a `Chart.yaml` as a chart.

**Flags**

| Flag                          | Default  | Description                                                                                       |
|-------------------------------|----------|---------------------------------------------------------------------------------------------------|
| `-f, --values <file>`         | —        | Values file to use. Repeat the flag to merge multiple files (later files win).                    |
| `-o, --output-format <fmt>`   | `pretty` | One of `pretty`, `json`, `yaml`, `junit`.                                                         |
| `-c, --config <path>`         | —        | Configuration file. CLI flags override values from the file.                                      |
| `-e, --environment <name>`    | —        | Use the `valuesFiles` defined under `environments.<name>` in the config file.                     |
| `--set key=val[,key=val…]`    | —        | Inline value override, identical in semantics to `helm template --set`. Repeatable.               |
| `--fail-on-error`             | `false`  | Exit with status `1` if any chart fails to render. Without this flag, errors are reported but ChartScan exits `0`. |

**Exit codes**

| Code | Meaning                                                                                |
|------|----------------------------------------------------------------------------------------|
| `0`  | All charts processed successfully, or errors were reported without `--fail-on-error`.  |
| `1`  | A fatal error occurred (bad flags, missing files), or `--fail-on-error` was set and at least one chart was invalid. |

---

## `template`

Render one or more Helm charts using `helm template`, writing the output to stdout or to a file.

**Synopsis**

```text
chartscan template [chart-path]... [flags]
```

At least one chart path is required. Multiple paths are allowed and are rendered in sequence.

**Flags**

| Flag                          | Default | Description                                                                              |
|-------------------------------|---------|------------------------------------------------------------------------------------------|
| `-f, --values <file>`         | —       | Values file to use. Repeat the flag to merge multiple files.                             |
| `-o, --output <file>`         | stdout  | Write the rendered manifests to this file instead of stdout.                             |
| `-c, --config <path>`         | —       | Configuration file. CLI flags override values from the file.                             |
| `-e, --environment <name>`    | —       | Use the `valuesFiles` defined under `environments.<name>` in the config file.            |
| `--set key=val[,key=val…]`    | —       | Inline value override, identical in semantics to `helm template --set`. Repeatable.      |

---

## `version`

Print the ChartScan version.

```bash
chartscan version
```

The version string is `dev` for `go run` and `go install` builds. Release builds inject the Git tag via `-ldflags "-X main.version=$VERSION"` (see [`.github/workflows/go-build.yml`](../.github/workflows/go-build.yml)).

---

## Output formats

The `-o, --output-format` flag on `scan` selects one of:

| Format   | Description                                                                                          |
|----------|------------------------------------------------------------------------------------------------------|
| `pretty` | Human-readable colored table. Default.                                                               |
| `json`   | One JSON document with the array of per-chart results. Suitable for piping into `jq`.                |
| `yaml`   | Same structure as `json` but YAML-encoded.                                                           |
| `junit`  | JUnit XML test report — one `<testcase>` per chart, with a `<failure>` element on rendering errors.  |

Each result entry contains the chart path, a success flag, any errors, the merged values, and the list of undefined value references.

---

## Recipes

**Scan one chart**

```bash
chartscan scan ./charts/my-chart -f values.yaml
```

**Scan every chart in a directory tree**

```bash
chartscan scan ./charts
```

**Merge multiple values files**

```bash
chartscan scan ./charts/my-chart \
  -f values.yaml \
  -f values-overrides.yaml
```

**Inline overrides**

```bash
chartscan scan ./charts/my-chart \
  -f values.yaml \
  --set image.tag=1.4.2,replicaCount=3
```

**Fail the build if any chart is broken**

```bash
chartscan scan ./charts --fail-on-error
```

**Render a chart to a file**

```bash
chartscan template ./charts/my-chart -f values.yaml -o rendered.yaml
```

**Render several charts in one invocation**

```bash
chartscan template ./charts/api ./charts/worker -f common-values.yaml
```

**Produce a JUnit report for CI**

```bash
chartscan scan ./charts -o junit > chartscan-report.xml
```

**List the environments declared in a config file**

```bash
chartscan -l -c chartscan.yaml
```

**Use a named environment from the config file**

```bash
chartscan scan -c chartscan.yaml -e staging
```
