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

# Resources per dependency tier, keyed by <kind>/<id>. The test rewrites
# terraform.tfvars.json between steps.
variable "tiers" {
  type = map(map(object({
    kind           = string
    resource_id    = string
    spec           = string
    api_version    = optional(string)
    input_revision = optional(string)
  })))
}

provider "gizclaw" {
  context = var.context
}

resource "gizclaw_resource" "t1" {
  for_each       = lookup(var.tiers, "t1", {})
  kind           = each.value.kind
  resource_id    = each.value.resource_id
  spec           = each.value.spec
  api_version    = each.value.api_version
  input_revision = each.value.input_revision
}

resource "gizclaw_resource" "t2" {
  for_each       = lookup(var.tiers, "t2", {})
  kind           = each.value.kind
  resource_id    = each.value.resource_id
  spec           = each.value.spec
  api_version    = each.value.api_version
  input_revision = each.value.input_revision
  depends_on     = [gizclaw_resource.t1]
}

resource "gizclaw_resource" "t3" {
  for_each       = lookup(var.tiers, "t3", {})
  kind           = each.value.kind
  resource_id    = each.value.resource_id
  spec           = each.value.spec
  api_version    = each.value.api_version
  input_revision = each.value.input_revision
  depends_on     = [gizclaw_resource.t2]
}

resource "gizclaw_resource" "t4" {
  for_each       = lookup(var.tiers, "t4", {})
  kind           = each.value.kind
  resource_id    = each.value.resource_id
  spec           = each.value.spec
  api_version    = each.value.api_version
  input_revision = each.value.input_revision
  depends_on     = [gizclaw_resource.t3]
}

resource "gizclaw_resource" "t5" {
  for_each       = lookup(var.tiers, "t5", {})
  kind           = each.value.kind
  resource_id    = each.value.resource_id
  spec           = each.value.spec
  api_version    = each.value.api_version
  input_revision = each.value.input_revision
  depends_on     = [gizclaw_resource.t4]
}
