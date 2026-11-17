#!/bin/sh

set -eu

if [ "$#" -ne 2 ]; then
	printf 'usage: %s DOCKERFILE GO_MOD\n' "$0" >&2
	exit 2
fi

dockerfile=$1
go_mod=$2

if [ ! -r "$dockerfile" ]; then
	printf 'Go version check failed: Dockerfile is not readable: %s\n' "$dockerfile" >&2
	exit 1
fi

if [ ! -r "$go_mod" ]; then
	printf 'Go version check failed: go.mod is not readable: %s\n' "$go_mod" >&2
	exit 1
fi

docker_go=$(awk '/^FROM golang:/ {version = $2; sub(/^golang:/, "", version); print version; exit}' "$dockerfile")
mod_toolchain=$(awk '$1 == "toolchain" {version = $2; sub(/^go/, "", version); print version; exit}' "$go_mod")

if [ -z "$docker_go" ] || [ -z "$mod_toolchain" ]; then
	printf 'Go version check failed: Dockerfile builder or go.mod toolchain is missing\n' >&2
	exit 1
fi

if [ "$docker_go" != "$mod_toolchain" ]; then
	printf 'Go version mismatch: Dockerfile uses %s, go.mod requires %s\n' "$docker_go" "$mod_toolchain" >&2
	exit 1
fi

printf 'Go version check passed: %s\n' "$docker_go"
