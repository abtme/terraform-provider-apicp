# One archive (an apicpd that makes one per account, or a single part):
terraform import apicp_account_backup.nightly acct_abc123/bkp_xyz789

# The archives of one per-domain backup, the settings archive first:
terraform import apicp_account_backup.nightly acct_abc123/bkp_settings,bkp_web,bkp_mail

# Every archive a backup run made (apicp keeps an account's latest 10 runs):
terraform import apicp_account_backup.nightly acct_abc123/arun_0123abcd
