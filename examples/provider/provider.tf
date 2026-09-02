terraform {
  required_providers {
    apicp = {
      source = "registry.terraform.io/abtme/apicp"
    }
  }
}

# url/token can also be set via the APICP_URL/APICP_TOKEN environment
# variables instead of hardcoding them here.
provider "apicp" {
  url   = "https://apicp.example.com"
  token = "apicp_..."
}
