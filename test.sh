#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

echo "==============================="
echo " ChartScan Quick Test Script"
echo "==============================="

echo ""
echo "[1/4] Formatting code..."
export PATH="/home/jaydee/sdk/go1.26.1/bin:$PATH"
export GOROOT="/home/jaydee/sdk/go1.26.1"
go fmt ./...

echo ""
echo "[2/4] Running Go Vet..."
go vet ./...

echo ""
echo "[3/4] Running Unit Tests..."
go test -v ./...

echo ""
echo "[4/4] Running Smoke Test (scanning mock/charts)..."
go run ./cmd/chartscan scan mock/charts

echo ""
echo "==============================="
echo " All checks passed! ✅"
echo "==============================="
