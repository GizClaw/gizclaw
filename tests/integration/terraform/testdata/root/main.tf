terraform {
  required_providers {
    gizclaw = {
      source = "gizclaw.local/gizclaw/gizclaw"
    }
  }
}

variable "context" {
  type = string
}

variable "catalog_sources" {
  type = list(string)
}

variable "product_sources" {
  type = list(string)
}

provider "gizclaw" {
  context = var.context
}

data "gizclaw_catalog" "selected" {
  sources         = var.catalog_sources
  product_sources = var.product_sources
}

locals {
  catalog = data.gizclaw_catalog.selected
  manifests = {
    for stage in [
      "credentials", "tenants", "voices", "models", "memory_layouts",
      "workflows", "firmwares", "runtime_profiles", "registration_tokens",
    ] : stage => { for identity, manifest in local.catalog[stage] : identity => jsondecode(manifest) }
  }
}

resource "gizclaw_resource" "credentials" {
  for_each    = local.manifests.credentials
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
}

resource "gizclaw_resource" "tenants" {
  for_each    = local.manifests.tenants
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
  depends_on  = [gizclaw_resource.credentials]
}

resource "gizclaw_resource" "voices" {
  for_each    = local.manifests.voices
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
  depends_on  = [gizclaw_resource.tenants]
}

resource "gizclaw_resource" "models" {
  for_each    = local.manifests.models
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
  depends_on  = [gizclaw_resource.tenants]
}

resource "gizclaw_resource" "memory_layouts" {
  for_each    = local.manifests.memory_layouts
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
}

resource "gizclaw_resource" "workflows" {
  for_each    = local.manifests.workflows
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
  depends_on  = [gizclaw_resource.memory_layouts]
}

resource "gizclaw_resource" "firmwares" {
  for_each    = local.manifests.firmwares
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
}

resource "gizclaw_resource" "runtime_profiles" {
  for_each    = local.manifests.runtime_profiles
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
  depends_on = [
    gizclaw_resource.models,
    gizclaw_resource.voices,
    gizclaw_resource.workflows,
    gizclaw_resource.memory_layouts,
  ]
}

resource "gizclaw_resource" "registration_tokens" {
  for_each    = local.manifests.registration_tokens
  kind        = each.value.kind
  resource_id = each.value.metadata.id
  spec        = jsonencode(each.value.spec)
  depends_on  = [gizclaw_resource.runtime_profiles, gizclaw_resource.firmwares]
}

output "overridden_ids" {
  value = local.catalog.overridden_ids
}

output "raids" {
  value = keys(local.catalog.raids)
}
