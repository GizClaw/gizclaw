# Terraform Provider

`terraform-provider-gizclaw` manages GizClaw Admin Resources declaratively with
Terraform. The provider connects to the Server's Admin service with the identity of a
local GizClaw CLI context; it does not run a local `gizclaw` executable. It is a pure Go
binary built with `CGO_ENABLED=0`, and every formal Release publishes macOS and Linux
builds.

| Item | Value |
| --- | --- |
| Provider source address | `gizclaw.local/gizclaw/gizclaw` |
| Provider version | Same as the GizClaw Release version, for example `0.17.6` for `v0.17.6` |
| Resource type | `gizclaw_resource` |
| Data source | `gizclaw_catalog` |
| Platforms | `darwin_amd64`, `darwin_arm64`, `linux_amd64`, `linux_arm64` |

`gizclaw.local` is not a public registry. Terraform installs this provider only from a
filesystem mirror or a local directory configured in `provider_installation`.

## Installation

Every Release contains four provider packages:

```text
terraform-provider-gizclaw_<version>_darwin_amd64.zip
terraform-provider-gizclaw_<version>_darwin_arm64.zip
terraform-provider-gizclaw_<version>_linux_amd64.zip
terraform-provider-gizclaw_<version>_linux_arm64.zip
```

Each zip contains exactly one executable, `terraform-provider-gizclaw_v<version>`. The
packages are listed in both `SHA256SUMS` and `release-manifest.json`. Verify the checksums,
then place the zips unchanged in a packed filesystem mirror:

```sh
version=0.17.6
mirror="$HOME/.terraform.d/gizclaw-mirror"
dir="$mirror/gizclaw.local/gizclaw/gizclaw"
mkdir -p "$dir"
gh release download "v$version" --repo GizClaw/gizclaw --dir "$dir" \
  --pattern "terraform-provider-gizclaw_${version}_*.zip" --pattern SHA256SUMS
(cd "$dir" && sha256sum --ignore-missing --check SHA256SUMS && rm SHA256SUMS)
```

On macOS without `sha256sum`, use `shasum -a 256 --ignore-missing --check SHA256SUMS`.

In the Terraform CLI configuration (`~/.terraformrc`, or the file named by
`TF_CLI_CONFIG_FILE`), install `gizclaw.local` only from that directory:

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/Users/me/.terraform.d/gizclaw-mirror"
    include = ["gizclaw.local/*/*"]
  }
  direct {
    exclude = ["gizclaw.local/*/*"]
  }
}
```

`path` must be absolute. Pin the provider version in the Terraform root:

```hcl
terraform {
  required_providers {
    gizclaw = {
      source  = "gizclaw.local/gizclaw/gizclaw"
      version = "0.17.6"
    }
  }
}
```

`terraform init` records the zip hash in `.terraform.lock.hcl`. To share one lock file across
platforms, put all four zips in the mirror and run
`terraform providers lock -fs-mirror=<mirror> -platform=darwin_arm64 -platform=linux_amd64 ...`.

Build the same packages from source:

```sh
build/build-terraform-provider.sh \
  --version 0.17.6 \
  --source-commit "$(git rev-parse HEAD)" \
  --source-epoch "$(git show -s --format=%ct HEAD)" \
  --output-dir .tmp/terraform-provider
```

The script needs `go`, `zip`, and `unzip`. It builds the four platforms with
`CGO_ENABLED=0` and needs no Git LFS objects, cgo toolchain, or Console build output.

## Provider configuration

```hcl
provider "gizclaw" {
  context  = "prod-admin"
  endpoint = "https://admin.example.com:443"
}
```

| Argument | Default | Meaning |
| --- | --- | --- |
| `context` | `GIZCLAW_CONTEXT`, then the CLI current context | Reads `identity.private-key` and `server.endpoint` from `$XDG_CONFIG_HOME/gizclaw/<context>/config.yaml` (`~/.config/gizclaw` when `XDG_CONFIG_HOME` is unset). |
| `endpoint` | `GIZCLAW_ENDPOINT`, then the context `server.endpoint` | Replaces the context Server endpoint for this provider process only; `config.yaml` is not modified. Must be `https://host[:port]` without userinfo, path, query, or fragment. |

Create contexts with `gizclaw context create` from the [CLI](./cli); the identity must be an
Admin identity of the target Server. The provider loads the context and validates the
endpoint during configuration, so a missing context, invalid identity, or invalid endpoint
fails immediately. A `context` or `endpoint` that is still unknown during plan is also an
error.

A second WebRTC connection with the same identity replaces the first one on the Server.
Each provider process therefore opens one long-lived connection per `context`/`endpoint`
pair, only when the first Server request is needed, and every resource operation reuses it
with at most 8 requests in flight. Connection or transport failures discard the connection
and retry with a new one, up to 3 attempts per operation; structured Admin API errors are not
retried. When Terraform cancels an operation, in-progress connection setup, reconnect waits, and requests stop. Do not use the same context from several Terraform or CLI processes at once.

## `gizclaw_resource`

```hcl
resource "gizclaw_resource" "openai" {
  kind        = "Credential"
  resource_id = "openai-main"
  spec = jsonencode({
    provider = "openai"
    body     = { api_key = "$${OPENAI_API_KEY}" }
  })
}
```

| Attribute | Kind | Description |
| --- | --- | --- |
| `id` | computed | `<kind>/<resource_id>`. |
| `api_version` | optional, computed | Only `gizclaw.admin/v1alpha1`; an omitted value keeps the stored one. Changing it replaces the resource. |
| `kind` | required | A Resource kind addressable by ID, for example `Credential` or `Model`; `ResourceList` and `<Kind>Resource` aliases are rejected. Changing it replaces the resource. |
| `resource_id` | required | Resource `metadata.id`, following the Admin API caller-defined ID rules (non-empty, no surrounding whitespace, at most 1024 characters, not `.` or `..`). Changing it replaces the resource. |
| `spec` | required, sensitive | A JSON object produced by `jsonencode(...)`. |
| `input_revision` | optional, sensitive | Lowercase 64-character SHA-256. Changing it re-applies an unchanged `spec`, for example after a write-only secret changes. |

Resource behavior:

- Create and Update prepare the manifest with the same rules as
  `gizclaw admin apply --file <file>.json`, apply it, and read it back. `${NAME}` and
  `${NAME:-default}` inside `spec` expand from the provider process environment; an unset
  variable without a default fails the apply. Write `$${NAME}` in HCL to produce a literal
  `${NAME}`.
- Read fetches the resource from the Server. Refreshes that arrive together are grouped into
  one batch read concurrently on the shared connection. A `<SCOPE>_NOT_FOUND` error code
  removes the resource from state; any other error keeps state and reports the error.
- On refresh, a Server `spec` that is semantically consistent with the configuration keeps
  the configured form; otherwise the Server value is recorded and the next plan shows the
  difference. `Credential` always keeps the configured `spec` because the Server does not
  return secrets. A string with environment placeholders, including `${NAME:-default}`, is
  consistent when its expansion under the apply rules equals the Server value.
- Delete calls Admin delete; an already absent resource counts as success.
- Import takes `<kind>/<resource_id>`:
  `terraform import gizclaw_resource.openai Credential/openai-main`. Import does not record
  `spec`; the next apply writes the configured `spec` to the Server.

The state schema version is 1. Version 0 state stores the resource ID as `name`; the provider
upgrades it to `resource_id` automatically.

The Terraform state of `gizclaw_resource` stores `spec`, which can contain secret placeholders
or plaintext secrets. Manage the state backend as secret storage.

## `gizclaw_catalog`

`gizclaw_catalog` resolves layered local manifest directories into the Admin Resources that
product definitions select. It only reads local files: it sends no Admin request and opens no
Server connection. The provider block still loads its context during configuration.

```hcl
data "gizclaw_catalog" "selected" {
  sources         = ["${path.root}/catalogs/upstream", "${path.root}/catalogs/overrides"]
  product_sources = ["${path.root}/products/default"]
}

resource "gizclaw_resource" "workflows" {
  for_each    = data.gizclaw_catalog.selected.workflows
  kind        = jsondecode(each.value).kind
  resource_id = jsondecode(each.value).metadata.id
  spec        = jsonencode(jsondecode(each.value).spec)
}
```

| Attribute | Kind | Description |
| --- | --- | --- |
| `sources` | required | Reusable catalog directories, lowest to highest precedence. |
| `product_sources` | required | Product directories containing RuntimeProfile and RegistrationToken manifests. |
| `credentials`, `tenants`, `voices`, `models`, `memory_layouts`, `workflows`, `firmwares`, `runtime_profiles`, `registration_tokens` | computed | Map from `<Kind>/<id>` to the selected manifest encoded as JSON. |
| `raids` | computed | Map from raid ID to the unchanged bytes of each selected `raid.json`. |
| `overridden_ids` | computed | Set of `<Kind>/<id>` defined by more than one entry of `sources`. |

A catalog source may contain `credentials/`, `tenants/`, `voices/`, `models/`,
`memory-layouts/`, `workflows/`, `firmwares/`, and `runtime-profiles/`; a product source may
contain only `runtime-profiles/` and `registration-tokens/`, with manifests of the matching
kind. Every `.yaml` and `.yml` file below these directories is read. Each manifest needs
`apiVersion`, `kind`, `metadata.id`, and `spec`; `metadata.id` follows the caller-defined ID
rules of `gizclaw_resource`, and `metadata.name` is rejected.

Resolution:

- A `<Kind>/<id>` in a later source replaces the one from an earlier source and is listed in
  `overridden_ids`. One source defining the same `<Kind>/<id>` twice, or a product source
  defining a `<Kind>/<id>` that already exists, fails the read.
- Selection starts from every product manifest. A RegistrationToken selects the RuntimeProfile
  in `spec.runtime_profile_id`, which may come from a catalog source, and the Firmware in
  `spec.firmware_id` when set. A RuntimeProfile selects the Workflows bound in
  `spec.workflows.collections`, the Models and Voices bound in `spec.resources.models` and
  `spec.resources.voices`, and the MemoryLayouts named by `spec.resources.memories.*.layout_id`.
  A Workflow selects the MemoryLayout in `spec.memory`, a Model or Voice selects the Tenant in
  `spec.provider.id`, and a Tenant selects the Credential in `spec.credential_id`.
- A `raid.json` below a catalog source's `workflows/` is selected when one of its
  `implementations.*.workflow_id` Workflows is selected; its `tester.workflow_id` Workflow is
  then selected as well. For the same raid ID, the later source wins.
- A missing or invalid reference fails the read, as does a selected RuntimeProfile with
  `spec.gameplay`, `spec.workflows.system`, or `spec.resources.pet_defs`, `game_defs`, or
  `badge_defs`, or a selected Workflow with `spec.driver` `pet`.

Each manifest value is compact JSON with the fields `apiVersion`, `kind`, `metadata.id`, and
`spec`, with object keys in `spec` sorted. `${NAME}` placeholders are kept as written;
`gizclaw_resource` expands them during apply. YAML is decoded with YAML 1.1 scalar rules, so
unquoted `yes` and `on` become `true`. Values are not sensitive; keep secrets in placeholders
rather than in catalog files.
