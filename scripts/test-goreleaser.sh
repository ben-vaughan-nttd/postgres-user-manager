#!/bin/sh

# Validate .goreleaser.yml configuration and run a full snapshot release inside
# the goreleaser-cross container image. Verifies:
#   - Staging copies only explicit release inputs (never untracked local
#     secrets or test files) into the temporary repository.
#   - goreleaser check passes with GITHUB_REPOSITORY_OWNER unset (defaulting to
#     "local") and rejects an invalid config.
#   - goreleaser release --snapshot --clean produces all 5 platform archives,
#     checksums.txt (15 entries: 5 archives + 10 SBOMs), 5 SPDX SBOMs, and
#     5 CycloneDX SBOMs.
#   - The built release Docker image targets linux/amd64, runs as non-root
#     appuser in read-only /app with root-owned 0755 binary, executes --help
#     and validate cleanly, and contains no repo config or Go source files.
#   - A second cached snapshot pass with mixed-case GITHUB_REPOSITORY_OWNER
#     normalizes the image tag to lowercase via tolower.
#
# Optional environment variables:
#   CROSS_IMAGE              Override goreleaser-cross image
#                            (default: ghcr.io/goreleaser/goreleaser-cross:v1.27)
#   GITHUB_REPOSITORY_OWNER  Optional owner name (normalized to lowercase in
#                            image tags; defaults to "local" when unset)

set -eu

CROSS_IMAGE=${CROSS_IMAGE:-ghcr.io/goreleaser/goreleaser-cross:v1.27}

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$repository_root/tmp"
temp_dir=$(mktemp -d "$repository_root/tmp/goreleaser-test-XXXXXX")
temp_repo="$temp_dir/repo"
example_config="$temp_dir/config.example.json"
mkdir -p "$temp_dir/gocache/build" "$temp_dir/gocache/mod"

syft_container=""
snapshot_tag=""
latest_tag=""
backup_latest_tag=""
backup_snapshot_tag=""
mixed_snapshot_tag=""
mixed_latest_tag=""
backup_mixed_latest_tag=""
backup_mixed_snapshot_tag=""
restore_test_tag_a=""
restore_test_tag_b=""
restore_test_tag_c=""
restore_test_bak_a=""
restore_test_bak_b=""
restore_test_img_a=""
restore_test_img_b=""
restore_test_img_c=""

inspect_image_id() {
	tag=$1
	[ -n "$tag" ] || return 0
	docker image inspect --format '{{.Id}}' "$tag" 2>/dev/null || true
}

pin_existing_tag() {
	tag=$1
	backup_tag=$2
	if [ -n "$(inspect_image_id "$tag")" ]; then
		docker tag "$tag" "$backup_tag"
		printf '%s\n' "$backup_tag"
	fi
}

restore_tag_ref() {
	tag=$1
	backup_tag=$2
	[ -n "$tag" ] || return 0
	if [ -n "$backup_tag" ]; then
		docker tag "$backup_tag" "$tag" >/dev/null 2>&1 || true
		docker rmi "$backup_tag" >/dev/null 2>&1 || true
	else
		docker rmi "$tag" >/dev/null 2>&1 || true
	fi
}

cleanup_overwritten_id() {
	post_id=$1
	shift
	[ -n "$post_id" ] || return 0
	for preserved_id in "$@"; do
		if [ -n "$preserved_id" ] && [ "$post_id" = "$preserved_id" ]; then
			return 0
		fi
	done
	docker rmi "$post_id" >/dev/null 2>&1 || true
}

restore_tag_set() {
	t1=$1; b1=$2
	t2=$3; b2=$4
	t3=$5; b3=$6
	t4=$7; b4=$8

	o1=$(inspect_image_id "$t1")
	o2=$(inspect_image_id "$t2")
	o3=$(inspect_image_id "$t3")
	o4=$(inspect_image_id "$t4")

	p1=$(inspect_image_id "$b1")
	p2=$(inspect_image_id "$b2")
	p3=$(inspect_image_id "$b3")
	p4=$(inspect_image_id "$b4")

	restore_tag_ref "$t1" "$b1"
	restore_tag_ref "$t2" "$b2"
	restore_tag_ref "$t3" "$b3"
	restore_tag_ref "$t4" "$b4"

	cleanup_overwritten_id "$o1" "$p1" "$p2" "$p3" "$p4"
	cleanup_overwritten_id "$o2" "$p1" "$p2" "$p3" "$p4"
	cleanup_overwritten_id "$o3" "$p1" "$p2" "$p3" "$p4"
	cleanup_overwritten_id "$o4" "$p1" "$p2" "$p3" "$p4"
}

cleanup() {
	if [ -n "$syft_container" ]; then
		docker rm -f "$syft_container" >/dev/null 2>&1 || true
	fi
	for test_tag in \
		"$restore_test_tag_a" \
		"$restore_test_tag_b" \
		"$restore_test_tag_c" \
		"$restore_test_bak_a" \
		"$restore_test_bak_b"
	do
		[ -n "$test_tag" ] && docker rmi "$test_tag" >/dev/null 2>&1 || true
	done
	for test_img in "$restore_test_img_a" "$restore_test_img_b" "$restore_test_img_c"; do
		[ -n "$test_img" ] && docker rmi "$test_img" >/dev/null 2>&1 || true
	done
	restore_tag_set \
		"$latest_tag" "$backup_latest_tag" \
		"$snapshot_tag" "$backup_snapshot_tag" \
		"$mixed_latest_tag" "$backup_mixed_latest_tag" \
		"$mixed_snapshot_tag" "$backup_mixed_snapshot_tag"
	chmod -R u+w "$temp_dir" 2>/dev/null || true
	if ! rm -rf "$temp_dir" 2>/dev/null; then
		docker run --rm -v "$temp_dir:/cleanup" --entrypoint sh "$CROSS_IMAGE" -c 'chmod -R u+w /cleanup 2>/dev/null || true; rm -rf /cleanup/* /cleanup/.[!.]* /cleanup/..?*' >/dev/null 2>&1 || true
		chmod -R u+w "$temp_dir" 2>/dev/null || true
		rm -rf "$temp_dir" 2>/dev/null || true
	fi
	rmdir "$repository_root/tmp" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

stage_release_inputs() {
	src=$1
	dst=$2
	mkdir -p "$dst"
	for top_file in .goreleaser.yml Dockerfile .dockerignore LICENSE README.md go.mod go.sum main.go; do
		cp "$src/$top_file" "$dst/$top_file"
	done
	(cd "$src" && find cmd internal -type f -name '*.go' ! -name '*_test.go' | sort) | while IFS= read -r rel_path; do
		mkdir -p "$dst/$(dirname -- "$rel_path")"
		cp "$src/$rel_path" "$dst/$rel_path"
	done
}

printf 'Running staging isolation self-test with planted sensitive files...\n'
raw_src="$temp_dir/raw-src"
stage_release_inputs "$repository_root" "$raw_src"
printf 'UNTRACKED_PROD_SECRET=1\n' > "$raw_src/.env.production"
printf 'UNTRACKED_PKCS12_SECRET=1\n' > "$raw_src/credentials.p12"
mkdir -p "$raw_src/internal/config"
printf 'UNTRACKED_NESTED_PKCS12_SECRET=1\n' > "$raw_src/internal/config/credentials.p12"
printf 'package config\n' > "$raw_src/internal/config/leak_test.go"

stage_release_inputs "$raw_src" "$temp_repo"
rm -rf "$raw_src"

for leaked in \
	.env.production \
	credentials.p12 \
	internal/config/credentials.p12 \
	internal/config/leak_test.go
do
	if [ -e "$temp_repo/$leaked" ]; then
		printf 'Staging isolation self-test failed: %s leaked into temp repo\n' "$leaked" >&2
		exit 1
	fi
done
if [ -n "$(find "$temp_repo" -name '*_test.go')" ]; then
	printf 'Staging isolation self-test failed: *_test.go leaked into temp repo\n' >&2
	exit 1
fi
printf 'Staging isolation self-test passed\n'

cp "$repository_root/config.example.json" "$example_config"

printf 'Initializing throwaway git repository...\n'
git -C "$temp_repo" init -q
git -C "$temp_repo" remote add origin https://github.com/local/postgres-user-manager.git
git -C "$temp_repo" add -A
git -C "$temp_repo" \
	-c core.hooksPath=/dev/null \
	-c commit.gpgsign=false \
	-c user.name=test \
	-c user.email=test@example.invalid \
	commit -q -m "test snapshot"
git -C "$temp_repo" \
	-c core.hooksPath=/dev/null \
	-c commit.gpgsign=false \
	-c tag.gpgsign=false \
	-c user.name=test \
	-c user.email=test@example.invalid \
	tag v0.0.0-test

printf 'Checking .goreleaser.yml with GITHUB_REPOSITORY_OWNER unset (local default)...\n'
docker run --rm \
	-e GIT_CONFIG_COUNT=1 \
	-e GIT_CONFIG_KEY_0=safe.directory \
	-e GIT_CONFIG_VALUE_0=/work \
	-v "$temp_repo:/work" \
	-w /work \
	"$CROSS_IMAGE" \
	check

printf 'Running negative self-test with invalid GoReleaser config...\n'
cat > "$temp_dir/goreleaser.invalid.yml" <<'EOF'
version: 2
not_a_valid_goreleaser_key: true
EOF
if docker run --rm \
	-e GIT_CONFIG_COUNT=1 \
	-e GIT_CONFIG_KEY_0=safe.directory \
	-e GIT_CONFIG_VALUE_0=/work \
	-v "$temp_repo:/work" \
	-v "$temp_dir/goreleaser.invalid.yml:/invalid.yml:ro" \
	-w /work \
	"$CROSS_IMAGE" \
	check -f /invalid.yml >/dev/null 2>&1; then
	printf 'GoReleaser negative self-test failed: invalid config was accepted\n' >&2
	exit 1
fi
printf 'GoReleaser config checks passed\n'

printf 'Running Docker tag restoration self-test...\n'
restore_test_tag_a="pum-tag-restore-a:$$"
restore_test_tag_b="pum-tag-restore-b:$$"
restore_test_tag_c="pum-tag-restore-c:$$"
restore_ctx="$temp_dir/restore-ctx"
mkdir -p "$restore_ctx"
printf 'FROM scratch\nLABEL pum.restore_test=a\n' > "$restore_ctx/Dockerfile.a"
printf 'FROM scratch\nLABEL pum.restore_test=b\n' > "$restore_ctx/Dockerfile.b"
printf 'FROM scratch\nLABEL pum.restore_test=c\n' > "$restore_ctx/Dockerfile.c"
docker build --load -q -f "$restore_ctx/Dockerfile.a" -t "$restore_test_tag_a" "$restore_ctx" >/dev/null
docker build --load -q -f "$restore_ctx/Dockerfile.b" -t "$restore_test_tag_b" "$restore_ctx" >/dev/null
restore_test_img_a=$(inspect_image_id "$restore_test_tag_a")
restore_test_img_b=$(inspect_image_id "$restore_test_tag_b")

# Case 1: tag_a (pre=img_a) overwritten with img_b while tag_b (pre=img_b) holds img_b.
restore_test_bak_a=$(pin_existing_tag "$restore_test_tag_a" "pum-tag-restore-bak-a:$$")
restore_test_bak_b=$(pin_existing_tag "$restore_test_tag_b" "pum-tag-restore-bak-b:$$")
docker tag "$restore_test_img_b" "$restore_test_tag_a"
restore_tag_set \
	"$restore_test_tag_a" "$restore_test_bak_a" \
	"$restore_test_tag_b" "$restore_test_bak_b" \
	"" "" \
	"" ""
restore_test_bak_a=""
restore_test_bak_b=""
if [ "$(inspect_image_id "$restore_test_tag_a")" != "$restore_test_img_a" ]; then
	printf 'Tag restoration self-test failed: %s was not restored to original ID\n' "$restore_test_tag_a" >&2
	exit 1
fi
if [ "$(inspect_image_id "$restore_test_tag_b")" != "$restore_test_img_b" ]; then
	printf 'Tag restoration self-test failed: %s was lost during sibling restore\n' "$restore_test_tag_b" >&2
	exit 1
fi

# Case 2: tag_a (pre=img_a) overwritten via docker build --load with fresh img_c
# (tests containerd pin survival and Phase 3 orphan cleanup) while new tag_c (pre="")
# points to img_b alongside preserved tag_b (pre=img_b).
restore_test_bak_a=$(pin_existing_tag "$restore_test_tag_a" "pum-tag-restore-bak-a:$$")
restore_test_bak_b=$(pin_existing_tag "$restore_test_tag_b" "pum-tag-restore-bak-b:$$")
docker build --load -q -f "$restore_ctx/Dockerfile.c" -t "$restore_test_tag_a" "$restore_ctx" >/dev/null
restore_test_img_c=$(inspect_image_id "$restore_test_tag_a")
docker tag "$restore_test_img_b" "$restore_test_tag_c"
restore_tag_set \
	"$restore_test_tag_a" "$restore_test_bak_a" \
	"$restore_test_tag_c" "" \
	"$restore_test_tag_b" "$restore_test_bak_b" \
	"" ""
restore_test_bak_a=""
restore_test_bak_b=""
if [ "$(inspect_image_id "$restore_test_tag_a")" != "$restore_test_img_a" ]; then
	printf 'Tag restoration self-test failed: %s was not restored after fresh overwrite\n' "$restore_test_tag_a" >&2
	exit 1
fi
if [ -n "$(inspect_image_id "$restore_test_tag_c")" ]; then
	printf 'Tag restoration self-test failed: newly created %s was not removed\n' "$restore_test_tag_c" >&2
	exit 1
fi
if [ "$(inspect_image_id "$restore_test_tag_b")" != "$restore_test_img_b" ]; then
	printf 'Tag restoration self-test failed: %s was lost when removing %s\n' "$restore_test_tag_b" "$restore_test_tag_c" >&2
	exit 1
fi
if [ -n "$(inspect_image_id "$restore_test_img_c")" ]; then
	printf 'Tag restoration self-test failed: orphaned overwritten image %s was not removed\n' "$restore_test_img_c" >&2
	exit 1
fi

docker rmi "$restore_test_tag_a" "$restore_test_tag_b" >/dev/null
restore_test_tag_a=""
restore_test_tag_b=""
restore_test_tag_c=""
restore_test_img_a=""
restore_test_img_b=""
restore_test_img_c=""
printf 'Docker tag restoration self-test passed\n'

printf 'Resolving Linux syft binary compatible with %s...\n' "$CROSS_IMAGE"
cross_arch=$(docker image inspect --format '{{.Architecture}}' "$CROSS_IMAGE")
syft_bin=""
if [ "$(uname -s)" = "Linux" ] && command -v syft >/dev/null 2>&1; then
	host_syft=$(command -v syft)
	candidate=$(readlink -f "$host_syft" 2>/dev/null || printf '%s' "$host_syft")
	if docker run --rm \
		-v "$candidate:/usr/local/bin/syft:ro" \
		--entrypoint /usr/local/bin/syft \
		"$CROSS_IMAGE" \
		--version >/dev/null 2>&1; then
		syft_bin="$candidate"
	fi
fi
if [ -z "$syft_bin" ]; then
	syft_container=$(docker create --platform "linux/$cross_arch" anchore/syft:latest)
	docker cp "$syft_container:/syft" "$temp_dir/syft"
	docker rm -f "$syft_container" >/dev/null
	syft_container=""
	chmod +x "$temp_dir/syft"
	syft_bin="$temp_dir/syft"
fi

if [ -n "${GITHUB_REPOSITORY_OWNER:-}" ]; then
	expected_owner=$(printf '%s' "$GITHUB_REPOSITORY_OWNER" | tr '[:upper:]' '[:lower:]')
else
	expected_owner="local"
fi

image_repo="ghcr.io/${expected_owner}/postgres-user-manager"
latest_tag="${image_repo}:latest"
snapshot_tag="${image_repo}:v0.0.0-test-next"
backup_latest_tag=$(pin_existing_tag "$latest_tag" "pum-goreleaser-bak-latest:$$")
backup_snapshot_tag=$(pin_existing_tag "$snapshot_tag" "pum-goreleaser-bak-snapshot:$$")

mixed_repo="ghcr.io/mixedcase-ownercheck/postgres-user-manager"
mixed_latest_tag="${mixed_repo}:latest"
mixed_snapshot_tag="${mixed_repo}:v0.0.0-test-next"
backup_mixed_latest_tag=$(pin_existing_tag "$mixed_latest_tag" "pum-goreleaser-bak-mixed-latest:$$")
backup_mixed_snapshot_tag=$(pin_existing_tag "$mixed_snapshot_tag" "pum-goreleaser-bak-mixed-snapshot:$$")

printf 'Running goreleaser release --snapshot --clean...\n'
if [ -n "${GITHUB_REPOSITORY_OWNER:-}" ]; then
	docker run --rm \
		-e GITHUB_REPOSITORY_OWNER \
		-e GOCACHE=/gocache/build \
		-e GOMODCACHE=/gocache/mod \
		-e GIT_CONFIG_COUNT=1 \
		-e GIT_CONFIG_KEY_0=safe.directory \
		-e GIT_CONFIG_VALUE_0=/work \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "$temp_dir/gocache:/gocache" \
		-v "$syft_bin:/usr/local/bin/syft:ro" \
		-v "$temp_repo:/work" \
		-w /work \
		"$CROSS_IMAGE" \
		release --snapshot --clean
else
	docker run --rm \
		-e GOCACHE=/gocache/build \
		-e GOMODCACHE=/gocache/mod \
		-e GIT_CONFIG_COUNT=1 \
		-e GIT_CONFIG_KEY_0=safe.directory \
		-e GIT_CONFIG_VALUE_0=/work \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "$temp_dir/gocache:/gocache" \
		-v "$syft_bin:/usr/local/bin/syft:ro" \
		-v "$temp_repo:/work" \
		-w /work \
		"$CROSS_IMAGE" \
		release --snapshot --clean
fi

printf 'Verifying GoReleaser dist artifacts...\n'
dist_dir="$temp_repo/dist"
if [ ! -f "$dist_dir/metadata.json" ]; then
	printf 'GoReleaser test failed: dist/metadata.json not found\n' >&2
	exit 1
fi

snapshot_version=$(tr -d '\n\r' < "$dist_dir/metadata.json" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
if [ -z "$snapshot_version" ]; then
	printf 'GoReleaser test failed: unable to parse version from dist/metadata.json\n' >&2
	exit 1
fi
snapshot_tag="${image_repo}:${snapshot_version}"
mixed_snapshot_tag="${mixed_repo}:${snapshot_version}"

if [ ! -s "$dist_dir/checksums.txt" ]; then
	printf 'GoReleaser test failed: checksums.txt missing or empty\n' >&2
	exit 1
fi

for archive in \
	"postgres-user-manager_${snapshot_version}_linux_amd64.tar.gz" \
	"postgres-user-manager_${snapshot_version}_linux_arm64.tar.gz" \
	"postgres-user-manager_${snapshot_version}_darwin_amd64.tar.gz" \
	"postgres-user-manager_${snapshot_version}_darwin_arm64.tar.gz" \
	"postgres-user-manager_${snapshot_version}_windows_amd64.zip"
do
	if [ ! -s "$dist_dir/$archive" ]; then
		printf 'GoReleaser test failed: missing or empty archive %s\n' "$archive" >&2
		exit 1
	fi
	if ! grep -Fq "  $archive" "$dist_dir/checksums.txt"; then
		printf 'GoReleaser test failed: checksum missing for archive %s\n' "$archive" >&2
		exit 1
	fi
done

for target in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64 windows_amd64; do
	spdx_name="postgres-user-manager_${snapshot_version}_${target}_sbom.spdx.json"
	cdx_name="postgres-user-manager_${snapshot_version}_${target}_sbom.cyclonedx.json"
	if [ ! -s "$dist_dir/$spdx_name" ]; then
		printf 'GoReleaser test failed: missing or empty SPDX SBOM %s\n' "$spdx_name" >&2
		exit 1
	fi
	if [ ! -s "$dist_dir/$cdx_name" ]; then
		printf 'GoReleaser test failed: missing or empty CycloneDX SBOM %s\n' "$cdx_name" >&2
		exit 1
	fi
	if ! grep -Fq "  $spdx_name" "$dist_dir/checksums.txt"; then
		printf 'GoReleaser test failed: checksum missing for SPDX SBOM %s\n' "$spdx_name" >&2
		exit 1
	fi
	if ! grep -Fq "  $cdx_name" "$dist_dir/checksums.txt"; then
		printf 'GoReleaser test failed: checksum missing for CycloneDX SBOM %s\n' "$cdx_name" >&2
		exit 1
	fi
done

checksum_lines=$(wc -l < "$dist_dir/checksums.txt" | tr -d ' ')
if [ "$checksum_lines" -ne 15 ]; then
	printf 'GoReleaser test failed: expected 15 checksum lines (5 archives + 10 SBOMs), got %s\n' "$checksum_lines" >&2
	exit 1
fi
printf 'GoReleaser archive, checksum, and SBOM verification passed\n'

printf 'Verifying GoReleaser snapshot Docker image (%s)...\n' "$snapshot_tag"
img_arch=$(docker image inspect --format '{{.Architecture}}' "$snapshot_tag")
if [ "$img_arch" != "amd64" ]; then
	printf 'GoReleaser image check failed: expected Architecture amd64, got %s\n' "$img_arch" >&2
	exit 1
fi
img_user=$(docker image inspect --format '{{.Config.User}}' "$snapshot_tag")
if [ "$img_user" != "appuser" ]; then
	printf 'GoReleaser image check failed: expected Config.User appuser, got %s\n' "$img_user" >&2
	exit 1
fi
img_workdir=$(docker image inspect --format '{{.Config.WorkingDir}}' "$snapshot_tag")
if [ "$img_workdir" != "/app" ]; then
	printf 'GoReleaser image check failed: expected Config.WorkingDir /app, got %s\n' "$img_workdir" >&2
	exit 1
fi

help_out=$(docker run --rm --platform=linux/amd64 "$snapshot_tag")
if ! printf '%s\n' "$help_out" | grep -Fq "Usage:"; then
	printf 'GoReleaser image check failed: --help output missing Usage text\n' >&2
	exit 1
fi

docker run --rm --platform=linux/amd64 --entrypoint sh "$snapshot_tag" -eu -c '
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
		echo "go toolchain must not be present in release image" >&2
		exit 1
	fi
'

docker run --rm --platform=linux/amd64 \
	-v "$example_config:/app/config.json:ro" \
	"$snapshot_tag" \
	validate --config /app/config.json >/dev/null

docker run --rm --platform=linux/amd64 --user 0 --entrypoint sh "$snapshot_tag" -eu -c '
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

printf 'GoReleaser snapshot Docker image verification passed\n'

printf 'Verifying mixed-case GITHUB_REPOSITORY_OWNER lowercase normalization...\n'
docker run --rm \
	-e GITHUB_REPOSITORY_OWNER=MixedCase-OwnerCheck \
	-e GOCACHE=/gocache/build \
	-e GOMODCACHE=/gocache/mod \
	-e GIT_CONFIG_COUNT=1 \
	-e GIT_CONFIG_KEY_0=safe.directory \
	-e GIT_CONFIG_VALUE_0=/work \
	-v /var/run/docker.sock:/var/run/docker.sock \
	-v "$temp_dir/gocache:/gocache" \
	-v "$temp_repo:/work" \
	-w /work \
	"$CROSS_IMAGE" \
	release --snapshot --clean --skip=before,sbom

if ! docker image inspect "$mixed_snapshot_tag" >/dev/null 2>&1; then
	printf 'GoReleaser mixed-case owner check failed: expected image %s not found\n' "$mixed_snapshot_tag" >&2
	exit 1
fi
if ! docker image inspect "$mixed_latest_tag" >/dev/null 2>&1; then
	printf 'GoReleaser mixed-case owner check failed: expected image %s not found\n' "$mixed_latest_tag" >&2
	exit 1
fi
printf 'Mixed-case owner lowercase normalization passed (%s)\n' "$mixed_snapshot_tag"
