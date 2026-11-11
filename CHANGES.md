# SAS_lsm 5 test builds

## 5.0.0-dev.3

`ssh-setup` output:

- Each host is now listed with a clear state:
  - `[ready]`: the key is already installed.
  - `[needs key]`: the key is not installed yet. This is expected on a first run, so it is no longer shown as an error.
  - `[problem]`: the host name can't be resolved, or the host or its sshd can't be reached.
  - `[host key]`: the host's SSH key has changed since it was first recorded.
- Before any passwords are requested, `ssh-setup` explains the next step: how many hosts need the key, and that each account's password is asked for once.
- Hosts with a problem that a password can't fix are skipped, with a suggested fix. You are no longer asked for their passwords.
- Each password prompt is labelled with the host and account (`==> [1/2] sas@host: enter the password for sas when prompted`).
- A summary at the end shows how many hosts are ready, were installed now, failed or were skipped. The exit code is 1 if any host is not ready.
- New `--verbose` option shows ssh's own message for each host that is not ready.
- If `ssh-copy-id` is not installed, `ssh-setup` says so before prompting and explains the manual steps.

Other:

- `doctor` shows the meaningful line of an SSH failure (it skips FIPS warnings).
- The end-to-end test covers `ssh-setup`, using fake `ssh`, `ssh-copy-id` and `ssh-keygen`.
- Documentation updated: SETUP.md and the Install and Usage Guide.

## 5.0.0-dev.2

- `ssh-setup` supports FIPS-enabled Linux:
  - It detects FIPS mode from the kernel flag or the crypto policy.
  - In FIPS mode it never uses or creates Ed25519 keys. It prefers ECDSA P-384, then RSA 4096.
- New `ssh-setup --hosts h1,user@h2 [--user U]`, which works without a configuration file.
- New `ssh-setup --key-type auto|ecdsa|rsa|ed25519`.
- Fix: `ssh-setup` no longer creates a key when every host is the local machine.

## 5.0.0-dev

- First test build of converted v.4.1.0 -> Shell to Go, with YAML target configuration files.
