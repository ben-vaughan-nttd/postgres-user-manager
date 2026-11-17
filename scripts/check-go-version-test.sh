#!/bin/sh

set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
check_script=$repository_root/scripts/check-go-version.sh
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM

"$check_script" "$repository_root/Dockerfile" "$repository_root/go.mod"

sed 's/^FROM golang:[^[:space:]]*/FROM golang:mismatch/' "$repository_root/Dockerfile" > "$test_dir/Dockerfile-mismatch"
if "$check_script" "$test_dir/Dockerfile-mismatch" "$repository_root/go.mod"; then
	printf 'Go version self-test failed: mismatch was accepted\n' >&2
	exit 1
fi
printf 'Go version mismatch self-test passed\n'

sed '/^toolchain /d' "$repository_root/go.mod" > "$test_dir/go.mod-missing"
if "$check_script" "$repository_root/Dockerfile" "$test_dir/go.mod-missing"; then
	printf 'Go version self-test failed: missing toolchain was accepted\n' >&2
	exit 1
fi
printf 'Go version missing-input self-test passed\n'
