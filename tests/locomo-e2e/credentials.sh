#!/usr/bin/env bash

locomo_e2e_credentials=(
  GIZCLAW_LOCOMO_E2E_EMBEDDING_API_KEY
  GIZCLAW_LOCOMO_E2E_MEM0_CUSTOM_INSTRUCTIONS_API_KEY
  GIZCLAW_LOCOMO_E2E_MEM0_DEFAULT_API_KEY
  GIZCLAW_LOCOMO_E2E_MODEL_API_KEY
  GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_ID
  GIZCLAW_LOCOMO_E2E_VOLC_ACCESS_KEY_SECRET
  GIZCLAW_LOCOMO_E2E_VOLC_API_KEY_ID
  GIZCLAW_LOCOMO_E2E_VOLC_MEM0_API_KEY
)

require_locomo_e2e_credentials() {
  local env_file="$1"
  local name value normalized lower
  local -a invalid=()

  if [[ ! -f "$env_file" ]]; then
    echo "missing credential file: $env_file" >&2
    return 2
  fi
  for name in "${locomo_e2e_credentials[@]}"; do
    unset "$name"
  done
  for name in "${locomo_e2e_credentials[@]}"; do
    value=""
    local matches=0 line value_length
    while IFS= read -r line || [[ -n "$line" ]]; do
      line="${line%$'\r'}"
      if [[ "$line" == "$name="* ]]; then
        value="${line#*=}"
        matches=$((matches + 1))
      fi
    done <"$env_file"
    if ((matches != 1)); then
      invalid+=("$name")
      continue
    fi
    value_length="${#value}"
    if ((value_length >= 2)) &&
      { [[ "${value:0:1}" == '"' && "${value:value_length-1:1}" == '"' ]] ||
        [[ "${value:0:1}" == "'" && "${value:value_length-1:1}" == "'" ]]; }; then
      value="${value:1:value_length-2}"
    fi
    normalized="${value//[[:space:]]/}"
    lower="$(printf '%s' "$normalized" | tr '[:upper:]' '[:lower:]')"
    case "$lower" in
      "" | *dummy* | *example* | *placeholder* | *replace* | *changeme*) invalid+=("$name") ;;
    esac
    export "$name=$value"
  done
  if ((${#invalid[@]} > 0)); then
    printf 'invalid or missing LoCoMo E2E credentials in %s:\n' "$env_file" >&2
    printf '  %s\n' "${invalid[@]}" >&2
    return 2
  fi
}
