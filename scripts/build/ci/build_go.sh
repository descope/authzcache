#!/usr/bin/env bash

BUILD_FOLDER=${1:-"./..."}
DO_NOT_MAKE=${2:-false}
WORKING_DIR=${3:-.}

echo "Building Go package: $BUILD_FOLDER"
echo "Working directory: $WORKING_DIR"

cd "$WORKING_DIR"

echo "Running go mod tidy..."
go mod tidy
if [ $? -ne 0 ]; then
    echo "go mod tidy failed"
    exit 1
fi

echo "Running go build..."
go build -v $BUILD_FOLDER
if [ $? -ne 0 ]; then
    echo "go build failed"
    exit 1
fi

echo "Build completed successfully"
