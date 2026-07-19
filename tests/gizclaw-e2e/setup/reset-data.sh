#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
e2e_dir="$(cd "$script_dir/.." && pwd)"
testdata_dir="$e2e_dir/testdata"
resource_dir="$testdata_dir/resources"
env_file="$e2e_dir/.env"
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

if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
fi

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

require_e2e_credentials() {
  local missing=()
  local placeholders=()
  local name
  for name in \
    GIZCLAW_E2E_DASHSCOPE_API_KEY \
    GIZCLAW_E2E_DOUBAO_API_KEY \
    GIZCLAW_E2E_DOUBAO_APP_ID \
    GIZCLAW_E2E_DOUBAO_SEARCH_API_KEY \
    GIZCLAW_E2E_GEMINI_API_KEY \
    GIZCLAW_E2E_MINIMAX_CN_API_KEY \
    GIZCLAW_E2E_MINIMAX_CN_APP_ID \
    GIZCLAW_E2E_MINIMAX_CN_GROUP_ID \
    GIZCLAW_E2E_MINIMAX_GLOBAL_API_KEY \
    GIZCLAW_E2E_MINIMAX_GLOBAL_APP_ID \
    GIZCLAW_E2E_MINIMAX_GLOBAL_GROUP_ID \
    GIZCLAW_E2E_OPENAI_API_KEY \
    GIZCLAW_E2E_VOLC_ARK_API_KEY \
    GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY \
    GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY_ID; do
    local value="${!name:-}"
    local normalized
    normalized="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
    if [[ -z "$value" ]]; then
      missing+=("$name")
    elif [[ "$normalized" == *dummy* || "$normalized" == *placeholder* || "$normalized" == *replace* || "$normalized" == *example* ]]; then
      placeholders+=("$name")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "missing required e2e credential env values:" >&2
    printf '  %s\n' "${missing[@]}" >&2
    echo "copy tests/gizclaw-e2e/.env.example to tests/gizclaw-e2e/.env and fill every required credential before reset-data init" >&2
    exit 2
  fi
  if [[ ${#placeholders[@]} -gt 0 ]]; then
    echo "placeholder e2e credential env values are not valid for reset-data init:" >&2
    printf '  %s\n' "${placeholders[@]}" >&2
    echo "replace placeholder values in tests/gizclaw-e2e/.env with real provider credentials before reset-data init" >&2
    exit 2
  fi
}

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

resource_names() {
  ruby -ryaml -e '
    def emit(resource)
      return unless resource.is_a?(Hash)
      kind = resource["kind"]
      if kind == "ResourceList"
        Array(resource.dig("spec", "items")).each { |item| emit(item) }
        return
      end
      name = resource.dig("metadata", "name")
      puts "#{kind}\t#{name}" if kind && name
    end
    ARGV.each { |path| emit(YAML.load_file(path)) }
  ' "$@"
}

delete_resource() {
  local kind="$1"
  local name="$2"

  local output status
  set +e
  output="$(run_gizclaw admin delete "$kind" "$name" --context "$admin_context" 2>&1)"
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
  done < <(resource_names "${files[@]}")

  local i kind name
  for ((i=${#resources[@]}-1; i>=0; i--)); do
    IFS=$'\t' read -r kind name <<<"${resources[$i]}"
    delete_resource "$kind" "$name"
  done
}

apply_resource() {
  local resource_file="$1"
  run_gizclaw admin apply --context "$admin_context" -f "$resource_file"
}

delete_firmware_artifact_if_exists() {
  local firmware_id="$1"
  local channel="$2"
  local output status
  set +e
  output="$(run_gizclaw admin firmwares delete-artifact "$firmware_id" --channel "$channel" --context "$admin_context" 2>&1)"
  status=$?
  set -e
  if [[ $status -eq 0 ]]; then
    return 0
  fi
  if [[ "$output" == *"FIRMWARE_ARTIFACT_NOT_FOUND"* || "$output" == *"RESOURCE_NOT_FOUND"* || "$output" == *"NOT_FOUND"* || "$output" == *"unexpected status 404"* ]]; then
    return 0
  fi
  printf '%s\n' "$output" >&2
  return "$status"
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

  if [[ "${GIZCLAW_E2E_SKIP_PROVIDER_SYNC:-0}" != "1" ]]; then
    run_gizclaw admin volc-tenants sync-voices volc-main --context "$admin_context" >/dev/null
  fi

  local firmware_id="devkit-firmware-main"
  local channel="stable"
  local asset_path="$testdata_dir/assets/firmware/devkit-firmware-main.tar"
  if [[ ! -f "$asset_path" ]]; then
    echo "missing firmware fixture asset: $asset_path" >&2
    exit 2
  fi
  delete_firmware_artifact_if_exists "$firmware_id" "$channel"
  run_gizclaw admin firmwares upload-artifact "$firmware_id" --channel "$channel" -f "$asset_path" --context "$admin_context" >/dev/null
}

if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  require_e2e_credentials
fi
if [[ "$mode" == "clear" || "$mode" == "reset" ]]; then
  clear_data
fi
if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  init_data
fi
