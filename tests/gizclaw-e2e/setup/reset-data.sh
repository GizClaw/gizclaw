#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
e2e_dir="$(cd "$script_dir/.." && pwd)"
testdata_dir="$e2e_dir/testdata"
resource_dir="$testdata_dir/resources"
env_file="$e2e_dir/.env"
# shellcheck source=credentials.sh
source "$script_dir/credentials.sh"
mode="reset"
admin_context_arg=""
config_home_arg=""
bin_arg=""

usage() {
  cat >&2 <<'EOF'
usage: reset-data.sh [clear|init|reset] [--context <admin-context>] [--config-home <dir>] [--bin <gizclaw>]

Seeds the e2e resource set through a GizClaw admin context. This is the host-side
entrypoint for Docker-backed setup servers and remote services.
EOF
}

while (($# > 0)); do
  case "$1" in
    clear|init|reset)
      mode="$1"
      shift
      ;;
    --context)
      if (($# < 2)); then
        usage
        exit 2
      fi
      admin_context_arg="$2"
      shift 2
      ;;
    --config-home)
      if (($# < 2)); then
        usage
        exit 2
      fi
      config_home_arg="$2"
      shift 2
      ;;
    --bin)
      if (($# < 2)); then
        usage
        exit 2
      fi
      bin_arg="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

selected_config_home="${GIZCLAW_E2E_CONFIG_HOME:-}"
selected_admin_context="${GIZCLAW_E2E_ADMIN_CONTEXT:-}"
selected_bin="${GIZCLAW_BIN:-}"

if [[ -n "$selected_config_home" ]]; then
  export GIZCLAW_E2E_CONFIG_HOME="$selected_config_home"
fi
if [[ -n "$selected_admin_context" ]]; then
  export GIZCLAW_E2E_ADMIN_CONTEXT="$selected_admin_context"
fi
if [[ -n "$selected_bin" ]]; then
  export GIZCLAW_BIN="$selected_bin"
fi

if [[ -n "$config_home_arg" ]]; then
  export GIZCLAW_E2E_CONFIG_HOME="$config_home_arg"
fi
if [[ -n "$admin_context_arg" ]]; then
  export GIZCLAW_E2E_ADMIN_CONTEXT="$admin_context_arg"
fi
if [[ -n "$bin_arg" ]]; then
  export GIZCLAW_BIN="$bin_arg"
fi

admin_context="${GIZCLAW_E2E_ADMIN_CONTEXT:-admin}"
gear1_context="${GIZCLAW_E2E_CMD_GEAR1_CONTEXT:-}"
gear2_context="${GIZCLAW_E2E_CMD_GEAR2_CONTEXT:-}"
config_home="${GIZCLAW_E2E_CONFIG_HOME:-}"

# Preserve Flowcraft runtime placeholders while admin apply expands provider
# credential placeholders from the setup environment.
export input='${input}'

bin_path() {
  if [[ -n "${GIZCLAW_BIN:-}" ]]; then
    echo "$GIZCLAW_BIN"
    return
  fi
  local default_bin="$testdata_dir/bin/gizclaw"
  if [[ ! -x "$default_bin" ]]; then
    "$e2e_dir/docker/setup/build.sh" >/dev/null
  fi
  echo "$default_bin"
}

run_gizclaw() {
  local bin
  bin="$(bin_path)"
  if [[ -n "$config_home" ]]; then
    XDG_CONFIG_HOME="$config_home" "$bin" "$@"
  else
    "$bin" "$@"
  fi
}

resource_files() {
  local resource_subdir
  while IFS= read -r resource_subdir; do
    find "$resource_subdir" -type f -name '*.yaml' -print | sort
  done < <(
    find "$resource_dir" -mindepth 1 -maxdepth 1 -type d -name '[0-9][0-9]-*' -print |
      sort
  )
}

resource_ids() {
  ruby -ryaml -e '
    def emit(resource)
      return unless resource.is_a?(Hash)
      kind = resource["kind"]
      if kind == "ResourceList"
        Array(resource.dig("spec", "items")).each { |item| emit(item) }
        return
      end
      id = resource.dig("metadata", "id")
      puts "#{kind}\t#{id}" if kind && id
    end
    ARGV.each { |path| emit(YAML.load_file(path)) }
  ' "$@"
}

delete_resource() {
  local kind="$1"
  local id="$2"

  local output status
  set +e
  output="$(run_gizclaw admin delete "$kind" "$id" --context "$admin_context" 2>&1)"
  status=$?
  set -e
  if [[ $status -eq 0 ]]; then
    return 0
  fi
  if [[ "$output" == *"RESOURCE_NOT_FOUND"* || "$output" == *"NOT_FOUND"* || "$output" == *"unexpected status 404"* ]]; then
    return 0
  fi
  printf '%s\n' "$output" >&2
  return "$status"
}

clear_data() {
  local files=()
  local resources=()
  local file
  while IFS= read -r file; do
    files+=("$file")
  done < <(resource_files)
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "no resource fixtures found in $resource_dir" >&2
    exit 2
  fi

  local line
  while IFS= read -r line; do
    resources+=("$line")
  done < <(resource_ids "${files[@]}")

  local i kind id
  for ((i=${#resources[@]}-1; i>=0; i--)); do
    IFS=$'\t' read -r kind id <<<"${resources[$i]}"
    delete_resource "$kind" "$id"
  done
}

apply_resource() {
  local resource_file="$1"
  run_gizclaw admin apply --context "$admin_context" -f "$resource_file"
}

init_data() {
  run_gizclaw connect set-name "E2E Admin" --context "$admin_context" >/dev/null
  if [[ -n "$gear1_context" ]]; then
    run_gizclaw connect set-name "Living Room Device" --context "$gear1_context" >/dev/null
  fi
  if [[ -n "$gear2_context" ]]; then
    run_gizclaw connect set-name "E2E Action Device" --context "$gear2_context" >/dev/null
  fi

  local files=()
  local file
  while IFS= read -r file; do
    files+=("$file")
  done < <(resource_files)
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "no resource fixtures found in $resource_dir" >&2
    exit 2
  fi

  for file in "${files[@]}"; do
    apply_resource "$file"
  done

  run_gizclaw admin volc-tenants sync-voices volc-main --context "$admin_context" >/dev/null
}

if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  require_gizclaw_e2e_credentials "$env_file"
fi
if [[ "$mode" == "clear" || "$mode" == "reset" ]]; then
  clear_data
fi
if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  init_data
fi
