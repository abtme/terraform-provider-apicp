# Changelog

## v0.2.0 (unreleased) - with apicp v0.7.0 (per-domain backups) and v0.8.0 (stored-mail compression)

### apicp_account_backup

- One backup can now be several archives: with per-domain backups apicp makes the account's settings archive
  plus a web and a mail archive per domain. New computed `backups` (each archive's `id`, `scope`, `domain`,
  `name`, `size_bytes`, `verified`, `verify_error`) and `run_id`.
- `backups[*].verified` is apicp's restore check of that archive (apicp v0.7.0 checks one-off backups too, when
  the server's `verify` setting is on), refreshed from the backup list on every read, not only at create; null
  when it was not checked or the apicpd does not report it. New `backups[*].verify_error` says why a check failed
  (a computed attribute of each archive, so no new top-level argument).
- `run_id` is the `batch_id` apicp v0.7.0 records on every archive of a "back up now" run (the same ID as the
  run). When the server reports batch IDs, the backup is every archive of the account with that batch ID; on
  older apicpd, and for archives made before batch IDs, it is the archives named in state, as before.
- `id` and `created_at` are those of the first archive (the settings archive); `size_bytes` is the size of all
  archives together. On an apicpd with one archive per account nothing changes: `backups` lists that archive.
- Destroying deletes every archive of the backup. Archives removed on the server (retention) drop out of
  `backups`; the resource is gone once none is left.
- Warnings about what an archive could not capture in full are shown after apply.
- A run that fails part way is an error, and on apicp v0.7.0 nothing of it is left: apicp deletes the archives
  it had made (`removed` in the run), so nothing goes into state. A part apicp could not delete stays on the
  server marked incomplete (apicp does not restore it) and is named in the error.
- A run whose archives were all made but failed apicp's restore check keeps them on the server
  (`verified = false`, with `verify_error`). Creating is then an error naming the archives that failed, and every
  archive the run left (this case, or incomplete leftovers above) is saved in state with the error, so Terraform
  marks the resource tainted: the next apply deletes those archives and takes a new backup, destroying deletes
  them, and `terraform untaint` keeps them.
- Import also takes `account_id/bkp_a,bkp_b,...` (the archives of one backup) and `account_id/arun_...` (every
  archive of a run). On apicp v0.7.0 the run's archives are found by `batch_id` in the backup list, so this works
  after apicp has pruned the run's status (it keeps an account's latest 10); older apicpd still needs the run.
  Archives of a failed run that are marked incomplete import with a warning; a run still going is refused.
  `account_id/backup_id` still works.

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
  sent. The `PUT` body holds only `enabled`, `destination`, `retention` and the optional `format`, `compress`,
  `encrypt`, `schedule_time`, `schedule_days`, `timezone` that are set - never a value read from the server.
- apicp v0.7.0 answers 422 to a setting it does not know (a provider newer than the server) on `PUT` and `PATCH`
  instead of ignoring it: that is now an error naming the argument ("this apicpd does not support ...; upgrade
  apicpd or remove the argument"), and no fallback is tried.

### apicp_database

- New optional `vhost_id`: links the database to the website that uses it, so it is backed up and restored with
  that website. Changing it only changes the link (`PATCH /v1/databases/{id}`, no replacement); `""` removes it;
  left out, the server's link (set by the importer or `apicp db link`) is kept and shown. On an apicpd without
  database links, create warns that the link was not made and an update is a clear error.
- Create sends only `engine`, `name` and (when set) `vhost_id`; an update sends only `vhost_id`. apicp v0.7.0's
  422 for a field it does not know reads as such, naming the argument, instead of as a refused link.

### apicp_account

- apicp's refusals read better: a name another account already has (409, names are unique ignoring case) says
  so and how to import that account; an invalid request (422), such as a parent of the wrong tier, explains the
  admin -> reseller -> user hierarchy.

### apicp_mail_domain

- New optional `compression` (`off`, the default, `zstd`, `lz4`, `gz`, `bz2`) and `compression_level` (`0`, the
  default, is the method's own default): how new mail of the domain is stored, through Dovecot's mail_compress
  plugin. Checked before anything is sent (unknown methods, xz, levels out of range, a level for lz4 or off);
  apicp itself refuses a method the server's Dovecot cannot write. Needs apicp v0.8.0: on an older apicpd, `off`
  works as before (nothing is sent) and any other method is an error after create (the domain is kept in state)
  or update.
- Compression is sent on update only when it changes: apicp refuses a compression change (409) while a
  compress-existing or decompress-existing job of the domain runs, and changes to the other arguments still go
  through then.
- New computed `compressed_mail`: some of the domain's stored mail may be compressed (apicp backups store it
  decompressed).
- Compressing or decompressing the mail already stored is a one-off job (`apicp mail compress-existing`,
  `decompress-existing`, the panel), deliberately not a resource.

### apicp_mail_settings

- New computed `compression_methods`: what the server's Dovecot can write for stored-mail compression (empty while
  compression is unavailable, and on apicp before v0.8.0). Compression itself is set per mail domain; there is no
  server-wide default.
- apicp v0.8.0 answers 422 to a mail settings field it does not know; the provider sends only the two it has
  always sent.
