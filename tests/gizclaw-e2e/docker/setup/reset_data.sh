#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../../.." && pwd)"
e2e_dir="$repo_root/tests/gizclaw-e2e"
testdata_dir="$e2e_dir/testdata"
workspace_dir="$testdata_dir/server-workspace"
resource_dir="$testdata_dir/resources"
bin_path="$testdata_dir/bin/gizclaw"
env_file="$e2e_dir/.env"
mode="${1:-reset}"
selected_config_home="${GIZCLAW_E2E_CONFIG_HOME:-}"

case "$mode" in
  clear|init|reset) ;;
  *)
    echo "usage: $0 [clear|init|reset]" >&2
    exit 2
    ;;
esac

if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
fi
if [[ -n "$selected_config_home" ]]; then
  export GIZCLAW_E2E_CONFIG_HOME="$selected_config_home"
fi

config_home="${GIZCLAW_E2E_CONFIG_HOME:-$testdata_dir/cmd-config-home}"
admin_context="${GIZCLAW_E2E_ADMIN_CONTEXT:-admin}"
gear1_context="${GIZCLAW_E2E_CMD_GEAR1_CONTEXT:-gear1}"
gear2_context="${GIZCLAW_E2E_CMD_GEAR2_CONTEXT:-gear2}"

# Preserve Flowcraft runtime placeholders while admin apply expands provider
# credential placeholders from the setup environment.
export input='${input}'

clear_data() {
  rm -rf "$workspace_dir/data" "$workspace_dir/gizclaw-server.log" "$workspace_dir/gizclaw-server.pid" "$workspace_dir/serve.pid"
  "$bin_path" migrate --workspace "$workspace_dir"
}

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
    local normalized="${value,,}"
    if [[ -z "$value" ]]; then
      missing+=("$name")
    elif [[ "$normalized" == *dummy* || "$normalized" == *placeholder* || "$normalized" == *replace* || "$normalized" == *example* ]]; then
      placeholders+=("$name")
    fi
  done

  if [[ ${#missing[@]} -gt 0 ]]; then
    echo "missing required e2e credential env values:" >&2
    printf '  %s\n' "${missing[@]}" >&2
    echo "copy tests/gizclaw-e2e/.env.example to tests/gizclaw-e2e/.env and fill every required credential before Docker e2e setup" >&2
    exit 2
  fi
  if [[ ${#placeholders[@]} -gt 0 ]]; then
    echo "placeholder e2e credential env values are not valid for Docker e2e setup:" >&2
    printf '  %s\n' "${placeholders[@]}" >&2
    echo "replace placeholder values in tests/gizclaw-e2e/.env with real provider credentials before Docker e2e setup" >&2
    exit 2
  fi
}

if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  require_e2e_credentials
fi

if [[ ! -x "$bin_path" ]]; then
  "$script_dir/build.sh" >/dev/null
fi

init_data() {
  XDG_CONFIG_HOME="$config_home" \
    "$bin_path" connect set-name "E2E Admin" --context "$admin_context" >/dev/null

  XDG_CONFIG_HOME="$config_home" \
    "$bin_path" connect set-name "Living Room Device" --context "$gear1_context" >/dev/null

  XDG_CONFIG_HOME="$config_home" \
    "$bin_path" connect set-name "E2E Action Device" --context "$gear2_context" >/dev/null

  local resource_files=()
  local resource_subdir
  while IFS= read -r resource_subdir; do
    while IFS= read -r resource_file; do
      resource_files+=("$resource_file")
    done < <(
      find "$resource_subdir" -type f -name '*.yaml' -print |
        sort
    )
  done < <(
    find "$resource_dir" -mindepth 1 -maxdepth 1 -type d -name '[0-9][0-9]-*' -print |
      sort
  )
  if [[ ${#resource_files[@]} -eq 0 ]]; then
    echo "no resource fixtures found in $resource_dir" >&2
    exit 2
  fi

  apply_resource() {
    local resource_file="$1"
    XDG_CONFIG_HOME="$config_home" \
      "$bin_path" admin apply --context "$admin_context" -f "$resource_file"
  }

  for resource_file in "${resource_files[@]}"; do
    apply_resource "$resource_file"
  done

  if [[ "${GIZCLAW_E2E_SKIP_PROVIDER_SYNC:-0}" != "1" ]]; then
    XDG_CONFIG_HOME="$config_home" \
      "$bin_path" admin volc-tenants sync-voices volc-main --context "$admin_context" >/dev/null
  fi

  upload_firmware_asset() {
    local firmware_id="devkit-firmware-main"
    local channel="stable"
    local asset_path="$repo_root/tests/gizclaw-e2e/testdata/assets/firmware/devkit-firmware-main.tar"
    if [[ ! -f "$asset_path" ]]; then
      echo "missing firmware fixture asset: $asset_path" >&2
      exit 2
    fi
    XDG_CONFIG_HOME="$config_home" \
      "$bin_path" admin firmwares upload-artifact "$firmware_id" --channel "$channel" -f "$asset_path" --context "$admin_context" >/dev/null
  }

  upload_firmware_asset

}

if [[ "$mode" == "clear" || "$mode" == "reset" ]]; then
  clear_data
fi
if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  init_data
fi
