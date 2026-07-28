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
# shellcheck source=../../setup/credentials.sh
source "$e2e_dir/setup/credentials.sh"
mode="${1:-reset}"
selected_config_home="${GIZCLAW_E2E_CONFIG_HOME:-}"

case "$mode" in
  clear|init|reset) ;;
  *)
    echo "usage: $0 [clear|init|reset]" >&2
    exit 2
    ;;
esac

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

if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  require_gizclaw_e2e_credentials "$env_file"
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

  local provider_voices_synced=0
  for resource_file in "${resource_files[@]}"; do
    # RuntimeProfiles in the gameplay fixtures bind provider-owned Voice
    # resources. Materialize those Voices after tenants exist and before the
    # first profile that references them is validated.
    if [[ "$provider_voices_synced" == "0" && "$resource_file" == */07-gameplay/* ]]; then
      XDG_CONFIG_HOME="$config_home" \
        "$bin_path" admin volc-tenants sync-voices volc-main --context "$admin_context" >/dev/null
      provider_voices_synced=1
    fi
    apply_resource "$resource_file"
  done

  if [[ "$provider_voices_synced" == "0" ]]; then
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
