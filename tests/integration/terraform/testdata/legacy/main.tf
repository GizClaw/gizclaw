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

variable "spec" {
  type = string
}

provider "gizclaw" {
  context = var.context
}

# Its state is written by the test with schema version 0, where the resource
# ID was stored as `name`.
resource "gizclaw_resource" "legacy" {
  kind        = "Tool"
  resource_id = "tf-legacy-tool"
  spec        = var.spec
}
