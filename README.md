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
| [`apicp_tls_certificate`](docs/resources/tls_certificate.md) | A Let's Encrypt (or other ACME CA) certificate for a `apicp_web_domain`. |
| [`apicp_ssh_access`](docs/resources/ssh_access.md) | Key-only SSH/SFTP login on a `apicp_web_domain`'s Unix user. |
| [`apicp_cron_job`](docs/resources/cron_job.md) | A scheduled command running as a `apicp_web_domain`'s Unix user. |
| [`apicp_dns_zone`](docs/resources/dns_zone.md) | A DNS zone on apicp's own PowerDNS deployment. |
| [`apicp_dns_record`](docs/resources/dns_record.md) | A DNS record within a `apicp_dns_zone`. |
| [`apicp_database`](docs/resources/database.md) | A MySQL/MariaDB or PostgreSQL database + admin user. |
| [`apicp_mail_domain`](docs/resources/mail_domain.md) | A mail domain (Postfix + Dovecot). |
| [`apicp_mailbox`](docs/resources/mailbox.md) | A mailbox within a `apicp_mail_domain`. |
| [`apicp_dkim`](docs/resources/dkim.md) | DKIM signing for a `apicp_mail_domain`. |
| [`apicp_antispam`](docs/resources/antispam.md) | Node-wide Rspamd toggle. |
| [`apicp_antivirus`](docs/resources/antivirus.md) | Node-wide ClamAV toggle. |
| [`apicp_smtp_relay`](docs/resources/smtp_relay.md) | A node's outbound SMTP smarthost/relay config. |
| [`apicp_firewall`](docs/resources/firewall.md) | Node-wide firewall enable/disable. |
| [`apicp_firewall_rule`](docs/resources/firewall_rule.md) | One firewall rule. |
| [`apicp_account`](docs/resources/account.md) | A tenancy account (admin/reseller/user). |
| [`apicp_package`](docs/resources/package.md) | A resource-limit template assignable to an account. |
| [`apicp_totp`](docs/resources/totp.md) | TOTP-gated token issuance for a `apicp_account`. |
| [`apicp_phpmyadmin`](docs/resources/phpmyadmin.md) | Control-plane phpMyAdmin toggle. |
| [`apicp_phppgadmin`](docs/resources/phppgadmin.md) | Control-plane phpPgAdmin toggle. |
| [`apicp_webmail`](docs/resources/webmail.md) | Control-plane Roundcube webmail toggle. |
| [`apicp_system_backup_config`](docs/resources/system_backup_config.md) | System-level backup scheduler enable/disable + destination. |

## Data sources

| Data source | Description |
|---|---|
| [`apicp_account_stats`](docs/data-sources/account_stats.md) | Resource counts (web domains, databases, mailboxes, etc.) for a `apicp_account` — read-only in the API itself, so a data source rather than a resource. |

## Deliberately not built

- **`apicp_web_domain_batch`** — a resource wrapping apicp's bulk
  vhost-creation endpoint (see apicp's own `PLAN.md` §5). Terraform's
  native `for_each`/`count` over `apicp_web_domain` already covers ordinary
  multi-instance usage; the batch endpoint's real value (one nginx reload
  per node per batch, rather than per item) isn't otherwise reachable from
  Terraform and is a real follow-up.
- **Node registry/enrollment** — apicp has no concept of a "create a node"
  API call; a node comes into existence by installing `apicp-agent` and
  running its own `enroll` flow against a one-time token
  (`POST /v1/nodes/enroll-tokens`). That token is itself single-use and
  consumed outside Terraform's lifecycle, so there's no clean resource
  shape here yet — `node_id` inputs elsewhere (e.g. `apicp_smtp_relay`)
  are plain string variables you fill in from `GET /v1/nodes` or the
  apicp desktop app, not a Terraform-managed reference.
- **Account tokens, account backups** — both are POST-only in apicp's own
  API (no GET of a specific token, no GET/PATCH of a specific backup, no
  DELETE for either) — an action, not a piece of drift-detectable
  configuration state, so there's nothing for Terraform's resource model
  to actually manage. Revisit if apicp's API grows real CRUD for these.

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
