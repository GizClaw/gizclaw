#!/usr/bin/env bash
# Terraform provider e2e against an isolated real Server: the provider is
# installed from a filesystem mirror and driven by the terraform CLI. It needs
# no model/provider credentials.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
terraform_bin="${GIZCLAW_E2E_TERRAFORM:-terraform}"
if ! command -v "$terraform_bin" >/dev/null 2>&1; then
	echo "terraform CLI not found: $terraform_bin (install Terraform or set GIZCLAW_E2E_TERRAFORM)" >&2
	exit 2
fi
cd "$repo_dir"
go test -tags gizclaw_e2e -count=1 -v -timeout 15m \
	-run '^TestTerraformProviderAppliesCatalogSelection$' \
	./tests/gizclaw-e2e/terraform
