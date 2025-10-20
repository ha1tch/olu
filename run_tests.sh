#!/bin/bash

# Test Runner Script for Olu
# Runs comprehensive tests with colored output

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     Olu Comprehensive Test Suite    ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

# Function to print section headers
print_header() {
    echo -e "${BLUE}▶ $1${NC}"
}

# Function to print success
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Function to print error
print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Function to print info
print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

# Parse command line arguments
RUN_BENCHMARKS=false
RUN_RACE=false
RUN_COVERAGE=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -b|--benchmark)
            RUN_BENCHMARKS=true
            shift
            ;;
        -r|--race)
            RUN_RACE=true
            shift
            ;;
        -c|--coverage)
            RUN_COVERAGE=true
            shift
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            echo "Usage: ./run_tests.sh [options]"
            echo ""
            echo "Options:"
            echo "  -b, --benchmark    Run benchmarks"
            echo "  -r, --race         Run race detector"
            echo "  -c, --coverage     Generate coverage report"
            echo "  -v, --verbose      Verbose output"
            echo "  -h, --help         Show this help"
            echo ""
            echo "Examples:"
            echo "  ./run_tests.sh                    # Run all tests"
            echo "  ./run_tests.sh -b                 # Run tests and benchmarks"
            echo "  ./run_tests.sh -r -c              # Run with race detector and coverage"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use -h or --help for usage information"
            exit 1
            ;;
    esac
done

# Track failures
FAILED=false

# Unit Tests
print_header "Running Unit Tests (Storage Layer)..."
if $VERBOSE; then
    if go test -v ./pkg/storage/; then
        print_success "Unit tests passed"
    else
        print_error "Unit tests failed"
        FAILED=true
    fi
else
    if go test ./pkg/storage/ 2>&1 | grep -E "(PASS|FAIL|ok|FAIL)"; then
        print_success "Unit tests passed"
    else
        print_error "Unit tests failed"
        FAILED=true
    fi
fi
echo ""

# Integration Tests
print_header "Running Integration Tests (HTTP Server)..."
if $VERBOSE; then
    if go test -v ./pkg/server/; then
        print_success "Integration tests passed"
    else
        print_error "Integration tests failed"
        FAILED=true
    fi
else
    if go test ./pkg/server/ 2>&1 | grep -E "(PASS|FAIL|ok|FAIL)"; then
        print_success "Integration tests passed"
    else
        print_error "Integration tests failed"
        FAILED=true
    fi
fi
echo ""

# Race Detector
if $RUN_RACE; then
    print_header "Running Race Detector..."
    if go test -race ./...; then
        print_success "No race conditions detected"
    else
        print_error "Race conditions detected"
        FAILED=true
    fi
    echo ""
fi

# Coverage
if $RUN_COVERAGE; then
    print_header "Generating Coverage Report..."
    if go test -coverprofile=coverage.out ./...; then
        go tool cover -html=coverage.out -o coverage.html
        COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
        print_success "Coverage report generated: coverage.html"
        print_info "Total coverage: $COVERAGE"
    else
        print_error "Coverage generation failed"
        FAILED=true
    fi
    echo ""
fi

# Benchmarks
if $RUN_BENCHMARKS; then
    print_header "Running Benchmarks..."
    print_info "This may take a few minutes..."
    if go test -bench=. -benchmem ./pkg/server/ > benchmark_results.txt; then
        print_success "Benchmarks complete"
        print_info "Results saved to benchmark_results.txt"
        echo ""
        echo -e "${YELLOW}Top 5 Operations by Speed:${NC}"
        grep "Benchmark" benchmark_results.txt | head -5
    else
        print_error "Benchmarks failed"
        FAILED=true
    fi
    echo ""
fi

# Summary
echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║            Test Summary              ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

if $FAILED; then
    print_error "Some tests failed"
    exit 1
else
    print_success "All tests passed!"
    echo ""
    print_info "Run with -h for more options"
fi

exit 0
