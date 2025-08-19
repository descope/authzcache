#!/usr/bin/env bash

CURRENT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

echo "build_test - Running tests"

export DB_AUTO_MIGRATE=false
go test -timeout 30m -v -coverpkg=./... -race -coverprofile=raw_coverage.out -covermode=atomic $@ ./...
EXIT_CODE=$? # Capture exit code
if [ $EXIT_CODE -ne 0 ]; then
	echo "build_test - exit with error: $EXIT_CODE - $@"
	exit 1
fi

echo "build_test - generating coverage.out"
cat raw_coverage.out | grep -v -e ".*\.pb.*\.go\:.*" | grep -v -e ".*\/main\.go\:.*" | grep -v -e ".*\/config\.go\:.*" | grep -v -e ".*\/.*mock.*\/.*\.go\:.*" | grep -v -e "\/test\/.*\.go\:.*" | grep -v -e "\/generated\/.*\.go\:.*" | grep -v -e "${1:-"empty"}" >coverage.out

echo "build_test - installing dave/courtney"
go install github.com/dave/courtney@master

echo "build_test - running courtney"
courtney -l coverage.out

echo "build_test - go tool cover"
go tool cover -func coverage.out | grep total | awk '{print $3}'

echo "build_test - go tool cover to get coverage.html"
go tool cover -html=coverage.out -o coverage.html
