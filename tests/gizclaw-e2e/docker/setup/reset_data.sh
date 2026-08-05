#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../../.." && pwd)"
e2e_dir="$repo_root/tests/gizclaw-e2e"
testdata_dir="$e2e_dir/testdata"
workspace_dir="$testdata_dir/server-workspace"
resource_dir="$testdata_dir/resources"
resource_paths="${GIZCLAW_E2E_RESOURCE_PATHS:-}"
sync_volc_tenant_id="${GIZCLAW_E2E_SYNC_VOLC_TENANT_ID-volc-main}"
bin_path="$testdata_dir/bin/gizclaw"
fixture_apply_bin="$testdata_dir/bin/gizclaw-e2e-fixture-apply"
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
  local resource_file resource_subdir
  if [[ -n "$resource_paths" ]]; then
    local relative_path
    for relative_path in $resource_paths; do
      if [[ "$relative_path" == /* || "$relative_path" == *..* || "$relative_path" != *.yaml ]]; then
        echo "invalid GIZCLAW_E2E_RESOURCE_PATHS entry: $relative_path" >&2
        exit 2
      fi
      resource_file="$resource_dir/$relative_path"
      if [[ ! -f "$resource_file" ]]; then
        echo "missing selected resource fixture: $resource_file" >&2
        exit 2
      fi
      resource_files+=("$resource_file")
    done
  else
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
  fi
  if [[ ${#resource_files[@]} -eq 0 ]]; then
    echo "no resource fixtures found in $resource_dir" >&2
    exit 2
  fi

  if [[ ! -x "$fixture_apply_bin" ]]; then
    (cd "$repo_root" && go build -o "$fixture_apply_bin" ./tests/gizclaw-e2e/internal/fixtureapply)
  fi
  local fixture_args=(
    --bin "$bin_path"
    --config-home "$config_home"
    --context "$admin_context"
  )
  if [[ -n "$sync_volc_tenant_id" ]]; then
    fixture_args+=(--sync-volc-tenant-id "$sync_volc_tenant_id")
  fi
  "$fixture_apply_bin" "${fixture_args[@]}" "${resource_files[@]}"

}

if [[ "$mode" == "clear" || "$mode" == "reset" ]]; then
  clear_data
fi
if [[ "$mode" == "init" || "$mode" == "reset" ]]; then
  init_data
fi
