#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
e2e_dir="$repo_root/test/gizclaw-e2e"
testdata_dir="$e2e_dir/testdata"
workspace_dir="$testdata_dir/server-workspace"
resource_dir="$testdata_dir/resources"
bin_path="$testdata_dir/bin/gizclaw"
env_file="${GIZCLAW_E2E_ENV:-$e2e_dir/.env}"
mode="${1:-reset}"

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

admin_setup_config_home="${GIZCLAW_E2E_ADMIN_SETUP_CONFIG_HOME:-$testdata_dir/admin-config-home}"
admin_setup_context="${GIZCLAW_E2E_ADMIN_SETUP_CONTEXT:-e2e-admin}"
default_client_config_home="$testdata_dir/gizclaw-config-home"
default_client_context="e2e-client"
client_config_home="${GIZCLAW_E2E_CLIENT_CONFIG_HOME:-$testdata_dir/gizclaw-config-home}"
client_context="${GIZCLAW_E2E_CLIENT_CONTEXT:-e2e-client}"

# Preserve Flowcraft runtime placeholders while admin apply expands provider
# credential placeholders from the setup environment.
export input='${input}'

if [[ ! -x "$bin_path" ]]; then
  "$script_dir/build.sh" >/dev/null
fi

clear_data() {
  "$script_dir/stop.sh" server >/dev/null || true
  rm -rf "$workspace_dir/data" "$workspace_dir/gizclaw-server.log" "$workspace_dir/gizclaw-server.pid"
  "$bin_path" migrate --workspace "$workspace_dir"
}

openai_ready() {
  [[ -n "${GIZCLAW_E2E_OPENAI_API_KEY:-}" ]]
}

volc_ready() {
  local name
  for name in \
    GIZCLAW_E2E_VOLC_APP_ID \
    GIZCLAW_E2E_VOLC_ARK_API_KEY \
    GIZCLAW_E2E_VOLC_ACCESS_KEY_ID \
    GIZCLAW_E2E_VOLC_SECRET_ACCESS_KEY \
    GIZCLAW_E2E_VOLC_TOKEN; do
    if [[ -z "${!name:-}" ]]; then
      return 1
    fi
  done
  return 0
}

minimax_cn_ready() {
  local name
  for name in \
    GIZCLAW_E2E_MINIMAX_CN_API_KEY \
    GIZCLAW_E2E_MINIMAX_CN_APP_ID \
    GIZCLAW_E2E_MINIMAX_CN_GROUP_ID; do
    if [[ -z "${!name:-}" ]]; then
      return 1
    fi
  done
  return 0
}

minimax_global_ready() {
  local name
  for name in \
    GIZCLAW_E2E_MINIMAX_GLOBAL_API_KEY \
    GIZCLAW_E2E_MINIMAX_GLOBAL_APP_ID \
    GIZCLAW_E2E_MINIMAX_GLOBAL_GROUP_ID; do
    if [[ -z "${!name:-}" ]]; then
      return 1
    fi
  done
  return 0
}

gemini_ready() {
  [[ -n "${GIZCLAW_E2E_GEMINI_API_KEY:-}" ]]
}

dashscope_ready() {
  [[ -n "${GIZCLAW_E2E_DASHSCOPE_API_KEY:-}" ]]
}

init_data() {
  "$script_dir/start-server.sh" >/dev/null

  XDG_CONFIG_HOME="$default_client_config_home" \
    "$bin_path" connect set-name "Living Room Device" --context "$default_client_context" >/dev/null
  if [[ "$client_config_home" != "$default_client_config_home" || "$client_context" != "$default_client_context" ]]; then
    XDG_CONFIG_HOME="$client_config_home" \
      "$bin_path" connect set-name "Living Room Device" --context "$client_context" >/dev/null
  fi

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
    local resource_key="${resource_file#$resource_dir/}"
    case "$resource_key" in
      00-credentials/00-openai.yaml|01-tenants/00-openai.yaml|03-models/00-openai-chat.yaml|90-acl/10-openai-credential-binding.yaml|90-acl/20-openai-chat-model-binding.yaml)
        if ! openai_ready; then
          return 0
        fi
        ;;
      00-credentials/01-volc.yaml|01-tenants/01-volc.yaml|03-models/01-volc-tts.yaml|03-models/02-volc-asr.yaml|03-models/03-doubao-realtime.yaml|03-models/04-doubao-lite-chat.yaml|03-models/05-volc-ast-translate.yaml|90-acl/11-volc-credential-binding.yaml|90-acl/21-volc-tts-model-binding.yaml|90-acl/22-volc-asr-model-binding.yaml|90-acl/23-volc-ast-translate-model-binding.yaml|90-acl/24-doubao-realtime-model-binding.yaml|90-acl/25-doubao-lite-chat-model-binding.yaml|90-acl/3*-volc-*-voice-binding.yaml)
        if ! volc_ready; then
          return 0
        fi
        ;;
      00-credentials/02-minimax-cn.yaml|01-tenants/02-minimax-cn.yaml|90-acl/12-minimax-cn-credential-binding.yaml)
        if ! minimax_cn_ready; then
          return 0
        fi
        ;;
      00-credentials/03-minimax-global.yaml|01-tenants/03-minimax-global.yaml|90-acl/13-minimax-global-credential-binding.yaml)
        if ! minimax_global_ready; then
          return 0
        fi
        ;;
      00-credentials/04-gemini.yaml|01-tenants/04-gemini.yaml|03-models/06-gemini-chat.yaml|90-acl/14-gemini-credential-binding.yaml|90-acl/26-gemini-chat-model-binding.yaml)
        if ! gemini_ready; then
          return 0
        fi
        ;;
      00-credentials/05-qwen-dashscope.yaml|01-tenants/05-qwen-dashscope.yaml|03-models/07-qwen-chat.yaml|90-acl/15-qwen-dashscope-credential-binding.yaml|90-acl/27-qwen-chat-model-binding.yaml)
        if ! dashscope_ready; then
          return 0
        fi
        ;;
    esac
    XDG_CONFIG_HOME="$admin_setup_config_home" \
      "$bin_path" admin apply --context "$admin_setup_context" -f "$resource_file"
  }

  for resource_file in "${resource_files[@]}"; do
    apply_resource "$resource_file"
  done

  upload_firmware_asset() {
    local firmware_id="devkit-firmware-main"
    local channel="stable"
    local bin="main"
    local asset_path="$resource_dir/assets/firmware/devkit-firmware-main.tar"
    if [[ ! -f "$asset_path" ]]; then
      echo "missing firmware fixture asset: $asset_path" >&2
      exit 2
    fi
    XDG_CONFIG_HOME="$admin_setup_config_home" \
      "$bin_path" admin firmwares upload-bin "$firmware_id" --channel "$channel" --bin "$bin" -f "$asset_path" --context "$admin_setup_context" >/dev/null
  }

  upload_firmware_asset

}

if [[ "$mode" == "clear" || "$mode" == "reset" ]]; then
  clear_data
fi
if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  init_data
fi
