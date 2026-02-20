#!/bin/bash

# Kubernetes MCP Server Test Runner
# This script runs all unit tests with coverage reporting

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_DIR="${PROJECT_ROOT}/coverage"
COVERAGE_FILE="${COVERAGE_DIR}/coverage.out"
COVERAGE_HTML="${COVERAGE_DIR}/coverage.html"

echo -e "${BLUE}🧪 Kubernetes MCP Server Test Suite${NC}"
echo "=================================================="

# Create coverage directory
mkdir -p "${COVERAGE_DIR}"

# Change to project root
cd "${PROJECT_ROOT}"

echo -e "${YELLOW}📦 Installing test dependencies...${NC}"
go mod tidy
go mod download

# Install testify if not already present
if ! go list -m github.com/stretchr/testify >/dev/null 2>&1; then
    echo -e "${YELLOW}📥 Installing testify...${NC}"
    go get github.com/stretchr/testify/assert
    go get github.com/stretchr/testify/require
    go get github.com/stretchr/testify/mock
fi

echo -e "${YELLOW}🔍 Running unit tests with coverage...${NC}"

# Run tests with coverage
go test -v -race -coverprofile="${COVERAGE_FILE}" -covermode=atomic ./tests/unit/...

# Check if tests passed
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✅ All tests passed!${NC}"
else
    echo -e "${RED}❌ Some tests failed!${NC}"
    exit 1
fi

# Generate coverage report
echo -e "${YELLOW}📊 Generating coverage report...${NC}"
go tool cover -html="${COVERAGE_FILE}" -o "${COVERAGE_HTML}"

# Display coverage summary
echo -e "${BLUE}📈 Coverage Summary:${NC}"
go tool cover -func="${COVERAGE_FILE}" | tail -1

# Calculate coverage percentage
COVERAGE_PERCENT=$(go tool cover -func="${COVERAGE_FILE}" | tail -1 | awk '{print $3}' | sed 's/%//')

# Check coverage threshold (80%)
THRESHOLD=80
if (( $(echo "$COVERAGE_PERCENT >= $THRESHOLD" | bc -l) )); then
    echo -e "${GREEN}✅ Coverage threshold met: ${COVERAGE_PERCENT}% >= ${THRESHOLD}%${NC}"
else
    echo -e "${YELLOW}⚠️  Coverage below threshold: ${COVERAGE_PERCENT}% < ${THRESHOLD}%${NC}"
fi

echo -e "${BLUE}📄 Coverage report generated: ${COVERAGE_HTML}${NC}"
echo -e "${BLUE}📄 Coverage data: ${COVERAGE_FILE}${NC}"

# Run linting if golangci-lint is available
if command -v golangci-lint &> /dev/null; then
    echo -e "${YELLOW}🔍 Running linter...${NC}"
    golangci-lint run ./...
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ Linting passed!${NC}"
    else
        echo -e "${YELLOW}⚠️  Linting issues found${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  golangci-lint not found, skipping linting${NC}"
fi

# Run security scan if gosec is available
if command -v gosec &> /dev/null; then
    echo -e "${YELLOW}🔒 Running security scan...${NC}"
    gosec ./...
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✅ Security scan passed!${NC}"
    else
        echo -e "${YELLOW}⚠️  Security issues found${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  gosec not found, skipping security scan${NC}"
fi

echo "=================================================="
echo -e "${GREEN}🎉 Test suite completed successfully!${NC}"
echo -e "${BLUE}📊 Open ${COVERAGE_HTML} in your browser to view detailed coverage report${NC}"
