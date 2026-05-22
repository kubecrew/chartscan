# Configuration

ChartScan can be driven entirely from the command line, but for repeatable runs — local development, CI pipelines, multi-environment promotions — you will want a `chartscan.yaml` file.

## Schema

```yaml
# Directory that contains your charts. Relative to the config file.
chartPath: ./charts

# Default output format for `scan`. One of: pretty, json, yaml, junit.
format: pretty

# Values files applied to every chart, unless overridden per environment
# or by the -f / --values CLI flag. Paths are relative to the config file.
valuesFiles:
  - values.yaml

# Optional named environments. Each environment overrides `valuesFiles`
# when the user passes -e <name>.
environments:
  test:
    valuesFiles:
      - values-test.yaml
  staging:
    valuesFiles:
      - values-staging.yaml
  production:
    valuesFiles:
      - values-production.yaml
```

All keys are optional. An empty file is valid; ChartScan will simply rely on CLI flags.

## Path resolution

Every path in `chartscan.yaml` — `chartPath` and every entry in `valuesFiles` — is resolved relative to the directory that holds the config file, not the current working directory. This means you can run ChartScan from any subdirectory of your repo without rewriting paths.

## Environments

Each entry under `environments` is a named bundle of `valuesFiles` to apply for that environment. Select one at runtime with `-e, --environment`:

```bash
chartscan scan -c chartscan.yaml -e staging
```

That replaces the top-level `valuesFiles` for the duration of the run. If the environment exists but defines no `valuesFiles`, the top-level list is cleared (no values files are passed).

List the environments declared in a file:

```bash
chartscan -l -c chartscan.yaml
```

Sample output:

```text
+-------------+---------------------------+
| ENVIRONMENT |       VALUES FILES        |
+-------------+---------------------------+
| test        | • values-test.yaml        |
| staging     | • values-staging.yaml     |
| production  | • values-production.yaml  |
+-------------+---------------------------+
```

## Automatic discovery in Git repositories

If you do not pass `-c`, ChartScan checks whether the current directory is inside a Git repository. If it is, ChartScan looks for `chartscan.yaml` at the repository root (the output of `git rev-parse --show-toplevel`). When the file is present, ChartScan prints:

```text
Using config file from project root: /path/to/repo/chartscan.yaml
```

…and proceeds as if `-c` had been passed. This makes shared team configurations friction-free: commit `chartscan.yaml` to your repo and every contributor gets the same behavior.

If you are not in a Git repository, or if the file does not exist at the repo root, ChartScan falls back to CLI-only configuration.

## CLI overrides

The order of precedence, lowest to highest:

1. `chartscan.yaml` defaults.
2. Environment override (`-e`) — replaces `valuesFiles`.
3. CLI flags — `-f, --values` replaces `valuesFiles`; `-o, --output-format` replaces `format`.
4. `--set` overrides — applied last when rendering, the same way `helm template --set` works.

In other words: the further to the right you go on the command line, the more it wins.
