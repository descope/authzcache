# Authz Cache

[![CI](https://github.com/descope/authzcache/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/descope/authzcache/actions/workflows/ci.yml)

## Getting Started
- Copy pre-commit hook: `cp -p .hooks/pre-commit-git .git/hooks/pre-commit`
- Setup local workspace as described [here](https://www.notion.so/descope/Go-local-environment-Setup-d8f4917ea299450a8599a21d0850e98d#1e65e79f383f4327924b70dad8a48b89)

## Generate new proto files
If you want to add new proto files or re-generate existings follow the [steps](https://www.notion.so/descope/Go-local-environment-Setup-d8f4917ea299450a8599a21d0850e98d#125f78ab95444d71a1f9cc0c0fc1defc)

## Start the grpc service
- `go mod tidy && go mod vendor` 
- `go run cmd/authzcache/main.go`
- Now you can use any [gRPC client](https://www.notion.so/descope/gRPC-clients-a7a9ff6af88e4d6aa5f6a10eb34edb04) to send requests to the service

## Build docker image locally
Follow the [steps](https://www.notion.so/descope/Go-local-environment-Setup-d8f4917ea299450a8599a21d0850e98d#89a337f609bb40ff9563fa6c7f06f152)
