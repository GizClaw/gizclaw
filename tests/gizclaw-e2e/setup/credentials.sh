#!/usr/bin/env bash

gizclaw_e2e_credentials=(
  GIZCLAW_E2E_DASHSCOPE_API_KEY
  GIZCLAW_E2E_DOUBAO_API_KEY
  GIZCLAW_E2E_DOUBAO_APP_ID
  GIZCLAW_E2E_DOUBAO_SEARCH_API_KEY
  GIZCLAW_E2E_GEMINI_API_KEY
  GIZCLAW_E2E_MINIMAX_CN_API_KEY
  GIZCLAW_E2E_MINIMAX_CN_APP_ID
  GIZCLAW_E2E_MINIMAX_CN_GROUP_ID
  GIZCLAW_E2E_MINIMAX_GLOBAL_API_KEY
  GIZCLAW_E2E_MINIMAX_GLOBAL_APP_ID
  GIZCLAW_E2E_MINIMAX_GLOBAL_GROUP_ID
  GIZCLAW_E2E_OPENAI_API_KEY
  GIZCLAW_E2E_VOLC_ARK_API_KEY
  GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_ID
  GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_SECRET
  GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY
  GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY_ID
)

require_gizclaw_e2e_credentials() {
  local env_file="$1"
  local name value normalized lower
  local -a invalid=()

  if [[ ! -f "$env_file" ]]; then
    echo "missing credential file: $env_file" >&2
    return 2
  fi

  for name in "${gizclaw_e2e_credentials[@]}"; do
    unset "$name"
  done
  for name in "${gizclaw_e2e_credentials[@]}"; do
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
      "" | *dummy* | *example* | *placeholder* | *replace* | *changeme*)
        invalid+=("$name")
        ;;
    esac
    export "$name=$value"
  done
  if ((${#invalid[@]} > 0)); then
    printf 'invalid or missing GizClaw E2E credentials in %s:\n' "$env_file" >&2
    printf '  %s\n' "${invalid[@]}" >&2
    return 2
  fi
}
