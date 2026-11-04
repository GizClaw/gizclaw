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

output "runtime_profiles" {
  value = keys(data.gizclaw_catalog.selected.runtime_profiles)
}
