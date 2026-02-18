# chart-down

Download and scan Helm charts for secrets using TruffleHog.

Given a Helm repository URL, chart-down fetches the repo index, downloads every chart archive, extracts it, and runs TruffleHog against the contents.

## Requirements

- Go 1.19+
- [TruffleHog](https://github.com/trufflesecurity/trufflehog) on PATH (`brew install trufflehog`)

## Install

```bash
go install github.com/RandomRobbieBF/chart-down@latest
```

Or build from source:

```bash
git clone https://github.com/RandomRobbieBF/chart-down.git
cd chart-down
go build -o chart-down .
```

## Usage

```bash
./chart-down -url <helm-repo-url> [-proxy <proxy-url>]
```

### Flags

| Flag | Required | Description |
|------|----------|-------------|
| `-url` | Yes | Helm repository URL (appends `/index.yaml` if needed) |
| `-proxy` | No | HTTP or SOCKS5 proxy URL (e.g. `http://127.0.0.1:8080`) |

### Examples

```bash
# Basic scan
./chart-down -url https://charts.example.com

# Scan through a proxy
./chart-down -url https://charts.example.com -proxy http://127.0.0.1:8080
```

## Output

| Path | Description |
|------|-------------|
| `charts.txt` | List of all chart download URLs |
| `charts-extracted/` | Extracted chart contents |
| `trufflehog-results/` | JSON findings per chart (only charts with secrets) |
| `trufflehog-results/verified-secrets.json` | Consolidated verified (confirmed active) secrets |

Review findings:

```bash
# Verified secrets only
cat trufflehog-results/verified-secrets.json 2>/dev/null | jq .

# All findings
cat trufflehog-results/*.json | jq .
```
