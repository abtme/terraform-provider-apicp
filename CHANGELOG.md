# Changelog

## Unreleased (to be released with apicp v0.7.0, per-domain backups)

### apicp_account_backup

- One backup can now be several archives: with per-domain backups apicp makes the account's settings archive
  plus a web and a mail archive per domain. New computed `backups` (each archive's `id`, `scope`, `domain`,
  `name`, `size_bytes`, `verified`) and `run_id`.
- `id` and `created_at` are those of the first archive (the settings archive); `size_bytes` is the size of all
  archives together. On an apicpd with one archive per account nothing changes: `backups` lists that archive.
- Destroying deletes every archive of the backup. Archives removed on the server (retention) drop out of
  `backups`; the resource is gone once none is left.
- Warnings about what an archive could not capture in full are shown after apply.
- A run that fails part way names the archives it did make (they stay on the server).
- Import also takes `account_id/bkp_a,bkp_b,...` (the archives of one backup) and `account_id/arun_...` (every
  archive of a run). `account_id/backup_id` still works.

### apicp_system_backup_config

- New optional `backup_unit` (`domain` or `account`) and `verify` (the restore check after each run), and
  computed `encryption_warning` and `last_run_warnings`.
- Only the settings in the configuration are written: on apicp v0.7.0 the provider uses
  `PATCH /v1/system/backup-config` with `enabled`, `destination` and the optional arguments that are set. A
  setting left out keeps what the server has saved (from the panel or `apicp backup configure`); removing an
  argument from the configuration puts that setting back to the server's default. Values read from the server
  are never written back (GET shows effective values, and saving them pinned `encrypt = false` on servers that
  got a key later).
- A warning is shown when the server has an encryption key but writes unencrypted backups, unless the
  configuration sets `encrypt = false`.
- `compress`, `encrypt`, `verify` and `backup_unit` set in the configuration are refreshed from the server, so a
  change made elsewhere shows as drift; settings the configuration leaves out stay null in state.
- Destroying sends only `enabled = false`; every other setting stays as it is.
- `retention` no longer defaults to `0`: left out, it keeps (and shows) the server's value. A configuration
  that relied on the default to reset retention to 0 must now set `retention = 0`.
- Older apicpd (no PATCH): the whole config is replaced with `PUT` as before, except that `retention`, when left
  out, keeps the server's value; `backup_unit` and `verify` are refused there with an error, before anything is
  sent.

### apicp_database

- New optional `vhost_id`: links the database to the website that uses it, so it is backed up and restored with
  that website. Changing it only changes the link (`PATCH /v1/databases/{id}`, no replacement); `""` removes it;
  left out, the server's link (set by the importer or `apicp db link`) is kept and shown. On an apicpd without
  database links, create warns that the link was not made and an update is a clear error.

### apicp_account

- apicp's refusals read better: a name another account already has (409, names are unique ignoring case) says
  so and how to import that account; an invalid request (422), such as a parent of the wrong tier, explains the
  admin -> reseller -> user hierarchy.
