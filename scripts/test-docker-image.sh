#!/bin/sh

# Validate Docker build context isolation, builder and runtime image layer
# hygiene, runtime hardening (non-root appuser, read-only /app and binary),
# and --target=release parity with the default source-built image.
#
# All work runs inside an isolated temporary copy under <repo>/tmp seeded with
# explicit repository inputs so any local developer files (.env, config.json,
# etc.) in the repository root remain untouched and are never staged.

set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$repository_root/tmp"
temp_dir=$(mktemp -d "$repository_root/tmp/docker-test-XXXXXX")
temp_repo="$temp_dir/repo"
ctx_dockerfile="$temp_dir/Dockerfile.ctx"

ctx_image="pum-docker-ctx-test:$$"
neg_ctx_image="pum-docker-neg-ctx-test:$$"
neg_env_ctx_image="pum-docker-neg-env-ctx-test:$$"
neg_subdir_ctx_image="pum-docker-neg-subdir-ctx-test:$$"
builder_image="pum-docker-builder-test:$$"
runtime_image="pum-docker-runtime-test:$$"
release_image="pum-docker-release-test:$$"
extract_container=""

cleanup() {
	if [ -n "$extract_container" ]; then
		docker rm -f "$extract_container" >/dev/null 2>&1 || true
	fi
	docker rmi -f \
		"$ctx_image" \
		"$neg_ctx_image" \
		"$neg_env_ctx_image" \
		"$neg_subdir_ctx_image" \
		"$builder_image" \
		"$runtime_image" \
		"$release_image" >/dev/null 2>&1 || true
	rm -rf "$temp_dir"
	rmdir "$repository_root/tmp" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

copy_repo() {
	src=$1
	dst=$2
	mkdir -p "$dst"
	for top_file in .dockerignore Dockerfile config.example.json go.mod go.sum main.go; do
		cp "$src/$top_file" "$dst/$top_file"
	done
	(cd "$src" && find cmd internal -type f -name '*.go' | sort) | while IFS= read -r rel_path; do
		mkdir -p "$dst/$(dirname -- "$rel_path")"
		cp "$src/$rel_path" "$dst/$rel_path"
	done
}

plant_sentinels() {
	target=$1
	printf 'SENTINEL_ENV_SECRET_26=root_env\n' > "$target/.env"
	printf 'SENTINEL_ENV_SECRET_26=root_env_local\n' > "$target/.env.local"
	mkdir -p "$target/cmd" "$target/internal/config" "$target/internal/database"
	printf 'SENTINEL_ENV_SECRET_26=nested_env\n' > "$target/internal/config/.env"
	printf '{"sentinel":"SENTINEL_CONFIG_SECRET_26_ROOT"}\n' > "$target/config.json"
	printf '{"sentinel":"SENTINEL_CONFIG_SECRET_26_NESTED"}\n' > "$target/internal/config/config.json"
	printf '{"sentinel":"SENTINEL_CONFIG_SECRET_26_EXTRA"}\n' > "$target/internal/config/secret.json"
	printf 'SENTINEL_PKCS12_SECRET_26\n' > "$target/internal/config/credentials.p12"
	printf 'SENTINEL_UNRECOGNIZED_SECRET_26\n' > "$target/cmd/extra.unrecognized"
	printf 'SENTINEL_DOCS_SECRET_26\n' > "$target/internal/database/NOTES.md"
	mkdir -p "$target/build" "$target/dist" "$target/.git" "$target/.vscode"
	printf 'SENTINEL_BUILD_SECRET_26\n' > "$target/build/sentinel"
	printf 'SENTINEL_DIST_SECRET_26\n' > "$target/dist/sentinel"
	printf 'SENTINEL_COVERAGE_SECRET_26\n' > "$target/coverage.out"
	printf 'SENTINEL_GIT_SECRET_26\n' > "$target/.git/sentinel"
	printf '{"sentinel":"SENTINEL_VSCODE_SECRET_26"}\n' > "$target/.vscode/settings.json"
	printf 'SENTINEL_BIN_SECRET_26\n' > "$target/postgres-user-manager"
}

verify_context() {
	ctx_dir=$1
	ctx_img=$2
	ctx_list="$temp_dir/ctx-files.txt"

	docker build --load -q -f "$ctx_dockerfile" -t "$ctx_img" "$ctx_dir" >/dev/null
	docker run --rm "$ctx_img" find /ctx -type f | sed 's|^/ctx/||' | sort > "$ctx_list"

	while IFS= read -r ctx_file; do
		[ -n "$ctx_file" ] || continue
		if ! printf '%s\n' "$ctx_file" | grep -Eq '^(go\.mod|go\.sum|main\.go|(cmd|internal)/.*\.go)$'; then
			printf 'Context check failed: disallowed file entered build context: %s\n' "$ctx_file" >&2
			return 1
		fi
		if printf '%s\n' "$ctx_file" | grep -Eq '(^|/)[^/]*_test\.go$'; then
			printf 'Context check failed: test file entered build context: %s\n' "$ctx_file" >&2
			return 1
		fi
	done < "$ctx_list"

	for required in \
		go.mod \
		go.sum \
		main.go \
		cmd/cmd.go \
		internal/config/config.go \
		internal/permissions/parse.go
	do
		if ! grep -Fxq "$required" "$ctx_list"; then
			printf 'Context check failed: required file missing: %s\n' "$required" >&2
			return 1
		fi
	done
}

scan_image_layers() {
	img=$1
	export_subdir=$2
	export_dir="$temp_dir/$export_subdir"
	image_tar="$temp_dir/${export_subdir}.tar"
	layer_list="$temp_dir/${export_subdir}-layers.txt"
	layer_files="$temp_dir/${export_subdir}-layer-files.txt"

	mkdir -p "$export_dir"
	docker save "$img" -o "$image_tar"
	tar -C "$export_dir" -xf "$image_tar"

	tr -d '\n\r' < "$export_dir/manifest.json" | awk '
	{
		line = $0
		while (match(line, /"Layers":\[[^]]*\]/)) {
			block = substr(line, RSTART, RLENGTH)
			line = substr(line, RSTART + RLENGTH)
			sub(/^"Layers":\[/, "", block)
			sub(/\]$/, "", block)
			n = split(block, parts, ",")
			for (i = 1; i <= n; i++) {
				gsub(/^[[:space:]]*"|"[[:space:]]*$/, "", parts[i])
				if (parts[i] != "") print parts[i]
			}
		}
	}
	' > "$layer_list"

	if [ ! -s "$layer_list" ]; then
		printf 'Layer scan failed (%s): no image layers found in manifest.json\n' "$img" >&2
		exit 1
	fi

	layer_count=0
	while IFS= read -r layer_rel; do
		layer_path="$export_dir/$layer_rel"
		if [ ! -f "$layer_path" ]; then
			printf 'Layer scan failed (%s): missing layer file %s\n' "$img" "$layer_rel" >&2
			exit 1
		fi
		layer_count=$((layer_count + 1))

		tar -tf "$layer_path" > "$layer_files"
		if grep -Eq '(^|/)(\.env(\.[^/]*)?|config\.json|config\.example\.json|secret\.json|credentials\.p12|extra\.unrecognized|NOTES\.md|coverage\.out|go\.mod|main\.go|[^/]*_test\.go|\.vscode(/|$)|build/sentinel$|dist/sentinel$|\.git/sentinel$)' "$layer_files"; then
			printf 'Layer scan failed (%s): forbidden filename found in layer %s\n' "$img" "$layer_rel" >&2
			exit 1
		fi

		layer_magic=$(od -An -tx1 -N2 "$layer_path" | tr -d '[:space:]')
		if [ "$layer_magic" = "1f8b" ]; then
			layer_reader="gzip -dc"
		else
			layer_reader="cat"
		fi

		if $layer_reader "$layer_path" | LC_ALL=C grep -Fq -f "$sentinel_tokens_file"; then
			matched_token=""
			while IFS= read -r token; do
				if $layer_reader "$layer_path" | LC_ALL=C grep -Fq "$token"; then
					matched_token=$token
					break
				fi
			done < "$sentinel_tokens_file"
			printf 'Layer scan failed (%s): sentinel token %s found in layer %s\n' "$img" "${matched_token:-unknown}" "$layer_rel" >&2
			exit 1
		fi
	done < "$layer_list"
	printf 'Image layer scan passed (%d layers inspected)\n' "$layer_count"
}

verify_runtime_image() {
	img=$1
	cfg=$2

	help_out=$(docker run --rm "$img")
	if ! printf '%s\n' "$help_out" | grep -Fq "Usage:"; then
		printf 'Runtime check failed: --help output missing Usage text for %s\n' "$img" >&2
		exit 1
	fi
	if ! printf '%s\n' "$help_out" | grep -Fq "postgres-user-manager"; then
		printf 'Runtime check failed: --help output missing binary name for %s\n' "$img" >&2
		exit 1
	fi

	docker run --rm --entrypoint sh "$img" -eu -c '
		uid=$(id -u)
		if [ "$uid" -eq 0 ]; then
			echo "expected non-root uid, got 0" >&2
			exit 1
		fi
		uname=$(id -un)
		if [ "$uname" != "appuser" ]; then
			echo "expected user appuser, got $uname" >&2
			exit 1
		fi
		cwd=$(pwd)
		if [ "$cwd" != "/app" ]; then
			echo "expected cwd /app, got $cwd" >&2
			exit 1
		fi
		if [ ! -r /app ] || [ ! -x /app ]; then
			echo "/app must be readable and searchable by appuser" >&2
			exit 1
		fi
		if [ -w /app ] || touch /app/.write-test 2>/dev/null; then
			echo "/app must not be writable by appuser" >&2
			exit 1
		fi
		bin=/usr/local/bin/postgres-user-manager
		if [ ! -x "$bin" ]; then
			echo "$bin must be executable by appuser" >&2
			exit 1
		fi
		if [ -w "$bin" ]; then
			echo "$bin must not be writable by appuser" >&2
			exit 1
		fi
		owner=$(stat -c "%u:%g" "$bin")
		if [ "$owner" != "0:0" ]; then
			echo "expected $bin owner 0:0, got $owner" >&2
			exit 1
		fi
		mode=$(stat -c "%a" "$bin")
		if [ "$mode" != "755" ]; then
			echo "expected $bin mode 755, got $mode" >&2
			exit 1
		fi
		if command -v go >/dev/null 2>&1; then
			echo "go toolchain must not be present in runtime image" >&2
			exit 1
		fi
	'

	docker run --rm --user 0 --entrypoint sh "$img" -eu -c '
		for dir in /app /src /root /usr/local; do
			[ -d "$dir" ] || continue
			found=$(find "$dir" \( \
				-name "*.go" -o \
				-name "go.mod" -o \
				-name ".env" -o \
				-name ".env.*" -o \
				-name "config.json" -o \
				-name "config.example.json" \
			\))
			if [ -n "$found" ]; then
				printf "unexpected files found in %s:\n%s\n" "$dir" "$found" >&2
				exit 1
			fi
		done
	'

	docker run --rm \
		-v "$cfg:/app/config.json:ro" \
		"$img" \
		validate --config /app/config.json >/dev/null
}

printf 'Preparing isolated repository copy with sentinel files...\n'
copy_repo "$repository_root" "$temp_repo"
plant_sentinels "$temp_repo"

sentinel_tokens_file="$temp_dir/sentinel-tokens.txt"
cat > "$sentinel_tokens_file" <<'EOF'
SENTINEL_ENV_SECRET_26
SENTINEL_CONFIG_SECRET_26
SENTINEL_CONFIG_SECRET_26_EXTRA
SENTINEL_PKCS12_SECRET_26
SENTINEL_UNRECOGNIZED_SECRET_26
SENTINEL_DOCS_SECRET_26
SENTINEL_BUILD_SECRET_26
SENTINEL_DIST_SECRET_26
SENTINEL_COVERAGE_SECRET_26
SENTINEL_GIT_SECRET_26
SENTINEL_VSCODE_SECRET_26
SENTINEL_BIN_SECRET_26
EOF

cat > "$ctx_dockerfile" <<'EOF'
FROM debian:trixie-slim
COPY . /ctx
EOF

printf 'Checking Docker build context allowlist and exclusions...\n'
verify_context "$temp_repo" "$ctx_image"
printf 'Build context check passed\n'

printf 'Running negative self-test without .dockerignore...\n'
neg_repo="$temp_dir/neg-repo"
mkdir -p "$neg_repo"
tar -C "$temp_repo" -cf - . | tar -C "$neg_repo" -xf -
rm -f "$neg_repo/.dockerignore"
if verify_context "$neg_repo" "$neg_ctx_image" 2>/dev/null; then
	printf 'Negative self-test failed: context check passed without .dockerignore\n' >&2
	exit 1
fi
printf 'Negative self-test (missing .dockerignore) passed\n'

printf 'Running negative self-test with !.env appended to .dockerignore...\n'
neg_env_repo="$temp_dir/neg-env-repo"
mkdir -p "$neg_env_repo"
tar -C "$temp_repo" -cf - . | tar -C "$neg_env_repo" -xf -
printf '\n!.env\n' >> "$neg_env_repo/.dockerignore"
if verify_context "$neg_env_repo" "$neg_env_ctx_image" 2>/dev/null; then
	printf 'Negative self-test failed: context check passed when !.env was appended\n' >&2
	exit 1
fi
printf 'Negative self-test (appended !.env) passed\n'

printf 'Running negative self-test with !internal/** appended to .dockerignore...\n'
neg_subdir_repo="$temp_dir/neg-subdir-repo"
mkdir -p "$neg_subdir_repo"
tar -C "$temp_repo" -cf - . | tar -C "$neg_subdir_repo" -xf -
printf '\n!internal/**\n' >> "$neg_subdir_repo/.dockerignore"
if verify_context "$neg_subdir_repo" "$neg_subdir_ctx_image" 2>/dev/null; then
	printf 'Negative self-test failed: context check passed when !internal/** was appended\n' >&2
	exit 1
fi
printf 'Negative self-test (appended !internal/**) passed\n'

printf 'Building builder stage with sentinels present and inspecting /src and /out...\n'
docker build --load --target=builder -t "$builder_image" "$temp_repo"
docker run --rm \
	-v "$sentinel_tokens_file:/tmp/sentinel-tokens.txt:ro" \
	--entrypoint sh "$builder_image" -eu -c '
	find /src -type f | sed "s|^/src/||" | sort > /tmp/src-files.txt
	if [ ! -s /tmp/src-files.txt ]; then
		echo "builder /src is unexpectedly empty" >&2
		exit 1
	fi
	while IFS= read -r src_file; do
		[ -n "$src_file" ] || continue
		if ! printf "%s\n" "$src_file" | grep -Eq "^(go\.mod|go\.sum|main\.go|(cmd|internal)/.*\.go)$"; then
			printf "unexpected file found in builder /src: %s\n" "$src_file" >&2
			exit 1
		fi
		if printf "%s\n" "$src_file" | grep -Eq "(^|/)[^/]*_test\.go$"; then
			printf "test file found in builder /src: %s\n" "$src_file" >&2
			exit 1
		fi
	done < /tmp/src-files.txt
	if LC_ALL=C grep -rFq -f /tmp/sentinel-tokens.txt /src /out; then
		matched_token=""
		while IFS= read -r token; do
			if LC_ALL=C grep -rFq "$token" /src /out; then
				matched_token=$token
				break
			fi
		done < /tmp/sentinel-tokens.txt
		printf "sentinel token %s found in builder /src or /out\n" "${matched_token:-unknown}" >&2
		exit 1
	fi
'
printf 'Builder stage /src and /out inspection passed\n'

printf 'Building default runtime image from source with sentinels present...\n'
docker build --load -t "$runtime_image" "$temp_repo"
scan_image_layers "$runtime_image" "runtime-export"

printf 'Verifying default runtime image as appuser...\n'
verify_runtime_image "$runtime_image" "$temp_repo/config.example.json"
printf 'Default runtime image verification passed\n'

printf 'Verifying --target=release parity...\n'
release_dir="$temp_dir/release-stage"
mkdir -p "$release_dir"
cp "$temp_repo/Dockerfile" "$release_dir/Dockerfile"
extract_container=$(docker create "$runtime_image")
docker cp "$extract_container:/usr/local/bin/postgres-user-manager" "$release_dir/postgres-user-manager"
docker rm -f "$extract_container" >/dev/null
extract_container=""

docker build --load --target=release -t "$release_image" "$release_dir"
verify_runtime_image "$release_image" "$temp_repo/config.example.json"
printf 'Release target parity verification passed\n'
