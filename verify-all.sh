#!/bin/bash
set -e

echo "==============================================="
echo "  TRIBUNAL: VERIFICATION & SIMULATION LAB"
echo "==============================================="

echo "1. Building Tribunal Core Engine..."
go build -o tribunal.exe ./cmd/tribunal
echo "✅ Core Engine Built."

echo "2. Building Verification CLI..."
go build -o verify-bundle.exe ./cmd/verify-bundle
echo "✅ Verification CLI Built."

echo "3. Running Core Unit Tests..."
go test ./...
echo "✅ Tests Passed."

echo "4. Checking formatting..."
if [ -n "$(gofmt -l .)" ]; then
    echo "❌ Some files are not formatted correctly. Run 'gofmt -w .'"
    exit 1
fi
echo "✅ Formatting OK."

echo "5. Checking for basic vet issues..."
go vet ./...
echo "✅ Vet OK."

echo "==============================================="
echo "🚀 ALL VERIFICATIONS PASSED"
echo "==============================================="
