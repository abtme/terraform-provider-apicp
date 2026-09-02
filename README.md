# Terraform Provider for apicp

A [Terraform](https://www.terraform.io) provider for
[apicp](https://github.com/abtme/apicp) — an API-only hosting control
panel (no web GUI; the API is the product). Manages web vhosts, TLS
certificates, DNS zones/records, databases, and mail domains/mailboxes
against apicp's own control-plane API.

## Using the provider

```hcl
terraform {
  required_providers {
    apicp = {
      source = "registry.terraform.io/abtme/apicp"
    }
  }
}

provider "apicp" {
  # url/token can also be set via the APICP_URL/APICP_TOKEN environment variables
  url   = "https://apicp.example.com"
  token = "apicp_..."
}

resource "apicp_web_domain" "example" {
  domain = "example.com"
}

resource "apicp_tls_certificate" "example" {
  vhost_id = apicp_web_domain.example.id
}
```

Full documentation, including the provider schema and resource import
syntax, is published on the
[Terraform Registry](https://registry.terraform.io/providers/abtme/apicp/latest/docs).

## Resources

| Resource | Description |
|---|---|
| [`apicp_web_domain`](docs/resources/web_domain.md) | An nginx-served web vhost. |
| [`apicp_tls_certificate`](docs/resources/tls_certificate.md) | A Let's Encrypt certificate for a `apicp_web_domain`. |
| [`apicp_dns_zone`](docs/resources/dns_zone.md) | A DNS zone on apicp's own PowerDNS deployment. |
| [`apicp_dns_record`](docs/resources/dns_record.md) | A DNS record within a `apicp_dns_zone`. |
| [`apicp_database`](docs/resources/database.md) | A MySQL/MariaDB or PostgreSQL database + admin user. |
| [`apicp_mail_domain`](docs/resources/mail_domain.md) | A mail domain (Postfix + Dovecot). |
| [`apicp_mailbox`](docs/resources/mailbox.md) | A mailbox within a `apicp_mail_domain`. |

Not yet built: `apicp_web_domain_batch` (a resource wrapping apicp's bulk
vhost-creation endpoint — see apicp's own `PLAN.md` §5). Terraform's
native `for_each`/`count` over `apicp_web_domain` already covers ordinary
multi-instance usage; the batch endpoint's real value (one nginx reload
per node per batch, rather than per item) isn't otherwise reachable from
Terraform and is a real follow-up.

## Developing the provider

Requires [Go](https://go.dev/) (see `go.mod` for the version) and
[Terraform](https://www.terraform.io/downloads) locally, plus a running
`apicpd` to test against (see apicp's own `NOTES.md`).

```shell
go build ./...
```

### Generating docs

Documentation under `docs/` is generated from the provider's schema plus
the example `.tf` files in `examples/`, via
[tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs):

```shell
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate
```

### Releasing

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which builds
and signs release artifacts with [GoReleaser](https://goreleaser.com/) and
publishes a GitHub Release. The Terraform Registry picks up new versions
automatically once connected.

## License

See [LICENSE](LICENSE).
