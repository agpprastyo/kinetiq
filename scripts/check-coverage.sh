#!/usr/bin/env bash
set -euo pipefail

MIN_COVERAGE=70.0

echo "=== Running Go Test with Coverage ==="
cd apps/api
go test -count=1 -coverprofile=coverage.out -coverpkg=./internal/... ./internal/... ./tests/...

TOTAL_COV=$(go tool cover -func=coverage.out | grep total: | awk '{print $3}' | tr -d '%')
echo "=== Total Go Test Coverage: ${TOTAL_COV}% (Threshold: ${MIN_COVERAGE}%) ==="

awk -v cov="$TOTAL_COV" -v min="$MIN_COVERAGE" 'BEGIN {
    if (cov < min) {
        printf("FAILED: Test coverage %s%% is below required threshold of %s%%\n", cov, min);
        exit 1;
    } else {
        printf("SUCCESS: Test coverage %s%% meets required threshold of %s%%\n", cov, min);
        exit 0;
    }
}'
