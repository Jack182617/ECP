#!/bin/sh

set -eu

case $0 in
  */*) script_parent=${0%/*} ;;
  *)
    printf '%s\n' "package-plugin.sh must be invoked by path" >&2
    exit 2
    ;;
esac

script_dir=$(CDPATH= cd -P -- "$script_parent" && pwd -P)
repo_root=$(CDPATH= cd -P -- "$script_dir/.." && pwd -P)
plugin_root="$repo_root/plugins/ecp-codex"
manifest_path="$plugin_root/.codex-plugin/plugin.json"
runtime_root="$plugin_root/runtime"

if [ ! -f "$repo_root/go.mod" ] || [ ! -f "$repo_root/cmd/ecp/main.go" ] || [ ! -f "$manifest_path" ]; then
  printf '%s\n' "repository or plugin layout is incomplete" >&2
  exit 2
fi

if [ -x /usr/bin/mktemp ]; then
  stage_root=$(/usr/bin/mktemp -d "$plugin_root/.runtime-package.XXXXXX")
elif [ -x /bin/mktemp ]; then
  stage_root=$(/bin/mktemp -d "$plugin_root/.runtime-package.XXXXXX")
else
  printf '%s\n' "fixed system mktemp is unavailable" >&2
  exit 1
fi

previous_runtime="$stage_root/previous-runtime"

cleanup() {
  cleanup_status=$?
  trap - EXIT HUP INT TERM

  if [ -e "$previous_runtime" ] && [ ! -e "$runtime_root" ]; then
    if ! mv -- "$previous_runtime" "$runtime_root"; then
      printf '%s\n' "could not restore the previous plugin runtime" >&2
      printf '%s\n' "previous runtime retained at $previous_runtime" >&2
      exit 1
    fi
  fi

  if ! rm -rf -- "$stage_root"; then
    printf '%s\n' "could not remove plugin runtime staging directory $stage_root" >&2
    if [ "$cleanup_status" -eq 0 ]; then
      cleanup_status=1
    fi
  fi
  exit "$cleanup_status"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

stage_runtime="$stage_root/runtime"
mkdir -p -- "$stage_runtime"

plugin_version=$(/usr/bin/sed -n 's/^[[:space:]]*"version": "\([^"]*\)",/\1/p' "$manifest_path")
if [ -z "$plugin_version" ]; then
  printf '%s\n' "plugin version could not be read" >&2
  exit 2
fi

targets='darwin/arm64 darwin/amd64 linux/arm64 linux/amd64'
manifest_entries=
separator=

for target in $targets; do
  target_os=${target%/*}
  target_arch=${target#*/}
  target_name="$target_os-$target_arch"
  target_dir="$stage_runtime/$target_name"
  target_binary="$target_dir/ecp"
  mkdir -p -- "$target_dir"

  (
    cd "$repo_root"
    env CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/ecp-package-go-cache}" \
      go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$target_binary" ./cmd/ecp
  )
  chmod 0755 "$target_binary"

  if [ -x /usr/bin/shasum ]; then
    checksum_output=$(/usr/bin/shasum -a 256 "$target_binary")
  elif [ -x /usr/bin/sha256sum ]; then
    checksum_output=$(/usr/bin/sha256sum "$target_binary")
  elif [ -x /bin/sha256sum ]; then
    checksum_output=$(/bin/sha256sum "$target_binary")
  else
    printf '%s\n' "fixed system SHA-256 executable is unavailable" >&2
    exit 1
  fi
  checksum=${checksum_output%% *}
  printf '%s\n' "$checksum" > "$target_dir/ecp.sha256"
  chmod 0644 "$target_dir/ecp.sha256"

  size=$(wc -c < "$target_binary" | tr -d '[:space:]')
  entry="{\"os\":\"$target_os\",\"arch\":\"$target_arch\",\"path\":\"runtime/$target_name/ecp\",\"sha256\":\"$checksum\",\"size_bytes\":$size}"
  manifest_entries="$manifest_entries$separator    $entry"
  separator=',
'
done

cat > "$stage_runtime/manifest.json" <<EOF
{
  "schema_version": 1,
  "plugin_name": "ecp-codex",
  "plugin_version": "$plugin_version",
  "artifacts": [
$manifest_entries
  ]
}
EOF
chmod 0644 "$stage_runtime/manifest.json"

if [ -e "$runtime_root" ]; then
  mv -- "$runtime_root" "$previous_runtime"
fi
if ! mv -- "$stage_runtime" "$runtime_root"; then
  printf '%s\n' "could not install the staged plugin runtime" >&2
  exit 1
fi

printf '%s\n' "Packaged ECP plugin runtime $plugin_version for: darwin-arm64 darwin-amd64 linux-arm64 linux-amd64"
