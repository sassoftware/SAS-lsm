# SAS_lsm 5 Setup and Usage Guide

SAS_lsm starts, stops and checks every tier of a SAS 9.4 deployment on UNIX/Linux in the correct order, from one command on one machine. Version 5 is a rewrite in Go. It ships as a single executable with no dependencies and uses a YAML configuration file.

## Contents

1. [How SAS_lsm works](#1-how-sas_lsm-works)
2. [Requirements](#2-requirements)
3. [Install](#3-install)
4. [Quick start](#4-quick-start)
5. [SSH setup](#5-ssh-setup)
6. [Configuration file reference](#6-configuration-file-reference)
7. [Start, stop and status behaviour](#7-start-stop-and-status-behaviour)
8. [Userexits](#8-userexits)
9. [Running SAS_lsm](#9-running-sas_lsm)
10. [Migrating from SAS_lsm 4.x](#10-migrating-from-sas_lsm-4x)
11. [Troubleshooting](#11-troubleshooting)

---

## 1. How SAS_lsm works

You describe your deployment as an ordered list of **tiers**, for example *metadata → compute → grid → middle tier*. A tier is one or more hosts plus the commands that start, stop and check them.

| Command | What happens |
|---|---|
| `start` | Checks that earlier tiers are UP and later tiers are DOWN, then starts each tier in order. Waits for each tier to report UP before moving on. |
| `stop` | Stops tiers in reverse order, waiting until each tier reports DOWN. |
| `status` | Reports UP, DOWN, PARTIAL or UNKNOWN for every tier and every host. |

SAS_lsm runs on one machine, called the **controller**. That can be one of the SAS hosts or a separate admin host. The controller reaches the other hosts with the standard OpenSSH client (`ssh`). **Nothing is installed on the other hosts.** They only need a POSIX `sh` and the SAS scripts that are already there, such as `sas.servers`.

```
                        ssh (one direction only)
 controller  ──────────────────────────────────────►  meta1, comp1, comp2, mid1 ...
 (sas_lsm + YAML)        as the SAS installer account   (runs sas.servers etc.)
```

---

## 2. Requirements

**Controller**

| Item | Requirement |
|---|---|
| Operating system | Linux (x86-64, ARM64, ppc64le), AIX 7.2+ (ppc64), or Solaris 11 (x86-64). FIPS-enabled Linux is supported. |
| Account | The SAS installer account (for example `sas`). SAS_lsm refuses to run as `root`. |
| Software | OpenSSH client (`ssh`, `ssh-keygen`, `ssh-copy-id`). Optional: `mailx` for e-mail, and `bash` to convert 4.x configurations. |
| Disk | A writable directory for reports. The default is `/var/tmp/sas_lsm`. |

**Tier hosts**

- An SSH server (`sshd`) the controller can reach on port 22, or on a port set in `~/.ssh/config`.
- The SAS installer account, with the controller's SSH key installed (see [section 5](#5-ssh-setup)).
- For `lsf-grid` tiers only: passwordless `sudo` for the LSF admin commands.

> **Solaris SPARC and HP-UX:** Go does not build for these platforms. You can still manage SAS there by putting the controller on a Linux host, because SAS_lsm only needs SSH access to the SAS hosts. Alternatively, keep using SAS_lsm 4.x on those systems.

---

## 3. Install

### Option A: release binary

Download the `sas_lsm` binary for your controller's platform from the project's GitHub releases. Copy it to a utilities directory owned by the SAS installer account:

```sh
mkdir -p /opt/sas/utilities/sas_lsm
cp sas_lsm-linux-amd64 /opt/sas/utilities/sas_lsm/sas_lsm
chmod 755 /opt/sas/utilities/sas_lsm/sas_lsm
/opt/sas/utilities/sas_lsm/sas_lsm version
```

Don't put it inside `<configdir>/Utilities`. SAS deployment tools manage that directory.

### Option B: build from source

This needs Go 1.21 or later. You can build every platform's binary from a single Linux machine.

```sh
git clone https://github.com/sassoftware/sas-lsm.git
cd sas-lsm
make build      # ./build/sas_lsm for this machine
make dist       # binaries for Linux, AIX and Solaris x86 in ./dist
make test e2e   # unit tests and end-to-end tests
```

SAS_lsm uses only the Go standard library. There are no third-party modules to download or review.

### Optional: shell setup

Add SAS_lsm to the installer account's `PATH` and set a default configuration file:

```sh
echo 'export PATH=/opt/sas/utilities/sas_lsm:$PATH'                 >> ~/.bash_profile
echo 'export SAS_LSM_CONFIG=/opt/sas/utilities/sas_lsm/prod.yaml'   >> ~/.bash_profile
```

---

## 4. Quick start

Run every step as the SAS installer account on the controller.

**Step 1: set up SSH to your SAS hosts.** List every host in the deployment. You type each account's password once.

```sh
sas_lsm ssh-setup --hosts meta1,comp1,comp2,mid1 --user sas
```

**Step 2: create a configuration.** Use whichever option fits your situation:

```sh
# a) Inspect the hosts and draft a configuration
sas_lsm discover --hosts meta1,comp1,comp2,mid1 --configdir /opt/sas/config/Lev1 --user sas > prod.yaml

# b) Convert an existing SAS_lsm 4.x configuration
sas_lsm convert /opt/sas/utilities/SAS_lsm.cfg > prod.yaml

# c) Start from an example
cp examples/distributed.yaml prod.yaml
```

**Step 3: review and validate.** Check the tier order, host names and `configdir`, then validate. `validate` reports every problem at once, with line numbers.

```sh
vi prod.yaml
sas_lsm validate -c prod.yaml
```

**Step 4: check every host.** If the configuration added hosts or accounts that step 1 didn't cover, run `sas_lsm ssh-setup -c prod.yaml` first. Hosts that are already set up are skipped.

```sh
sas_lsm doctor -c prod.yaml
```

**Step 5: preview the plan, then run it.** `--dry-run` shows what would happen without running anything.

```sh
sas_lsm start -c prod.yaml --dry-run
sas_lsm status -c prod.yaml
sas_lsm stop -c prod.yaml
sas_lsm start -c prod.yaml
```

On a single-machine deployment you can skip step 1 and step 4's SSH checks. See `examples/single-machine.yaml`.

---

## 5. SSH setup

### What is required and why

The SAS_lsm 4.x documentation asked for "multi-directional" passwordless SSH. **SAS_lsm 5 only needs SSH in one direction:** from the controller to each tier host, as that tier's `user`. Tier hosts never connect to each other or back to the controller. This holds for `lsf-grid` tiers too, because the controller drives every grid node directly.

SSH keys are required because SAS servers are started without a person present, often from cron or at boot. The controller therefore logs in with a key instead of a password.

SAS_lsm skips SSH when a tier host is the controller itself **and** the tier user is the user running `sas_lsm`. It recognizes the controller by its short host name, or by `localhost`. If the tier user is different (for example, a separate middle-tier account), SAS_lsm uses SSH even on the local machine so the command runs as the correct account.

### Automated setup (recommended)

You can give `ssh-setup` a list of hosts, or a configuration file:

```sh
sas_lsm ssh-setup --hosts meta1,comp1,mid1 --user sas   # before a configuration exists
sas_lsm ssh-setup --hosts sas@meta1,sasmid@mid1         # user@host sets the account per host
sas_lsm ssh-setup -c prod.yaml                          # every user@host in the configuration
```

`ssh-setup` does the following:

1. **Chooses a key.** It uses an existing key of an allowed type in `~/.ssh`, or creates one without a passphrase (see [Key types and FIPS mode](#key-types-and-fips-mode)).
2. **Checks each host with that key** and lists it with one of these states:

   | State | Meaning | What happens next |
   |---|---|---|
   | `[ready]` | The key is already installed. | Nothing; the host is skipped. |
   | `[needs key]` | The host is reachable, but the key isn't installed yet. This is expected the first time. | You are asked for the account's password once, and the key is installed. |
   | `[problem]` | The host name can't be resolved, or the host or its `sshd` can't be reached. | Skipped, with a suggested fix. A password would not help. |
   | `[host key]` | The host's SSH key has changed since it was first recorded. | Skipped. Confirm the change is expected, run `ssh-keygen -R <host>`, then run `ssh-setup` again. |

3. **Explains the next step** before asking for anything: how many hosts need the key, and that each account's password is requested once.
4. **Installs the key** with `ssh-copy-id`, one host at a time. Each prompt names the host and account.
5. **Verifies** each host and prints a summary.

Example of a first run:

```
Using existing SSH key /home/sas/.ssh/id_ecdsa.pub

Checking which hosts already accept this key...

  [ready]      sas@meta1   key already installed
  [needs key]  sas@comp1   key not installed yet (expected)
  [needs key]  sas@mid1    key not installed yet (expected)

2 hosts need the key. For each one, ssh-copy-id will ask for that account's
password once, then install the key so SAS_lsm can log in without a password.
Continue? [y/N] y

==> [1/2] sas@comp1: enter the password for sas when prompted
sas@comp1's password:
Number of key(s) added: 1
    verified: sas@comp1 now accepts the key

==> [2/2] sas@mid1: enter the password for sas when prompted
sas@mid1's password:
Number of key(s) added: 1
    verified: sas@mid1 now accepts the key

Summary
  ready:    3 host(s) (1 already set up, 2 installed now)

All hosts are ready.
```

Running `ssh-setup` again is safe. Hosts that are ready are skipped, so you can rerun it after fixing a problem host.

| Option | Meaning |
|---|---|
| `--hosts h1,user@h2` | Hosts to set up. Use instead of `-c`. |
| `--user U` | Account for hosts listed without `user@`. Default: the account running `sas_lsm`. |
| `--key-type T` | `auto` (default), `ecdsa`, `rsa` or `ed25519`. |
| `--yes` | Don't ask before installing keys. |
| `--verbose` | Also show ssh's own message for each host that isn't ready. |

`ssh-setup` exits with code 0 when every host is ready, and 1 if any host failed or was skipped.

### Key types and FIPS mode

`ssh-setup` checks whether the controller is in FIPS mode. It reads `/proc/sys/crypto/fips_enabled` and, on RHEL 8 and later, the system crypto policy. FIPS mode doesn't allow Ed25519 keys, so key selection depends on the mode:

| `--key-type` | Not in FIPS mode | In FIPS mode |
|---|---|---|
| `auto` (default) | Uses or creates Ed25519, then ECDSA, then RSA | Uses or creates ECDSA (P-384), then RSA (4096 bits). Ed25519 is never used. |
| `ecdsa` / `rsa` | That type | That type |
| `ed25519` | Ed25519 | Refused with an error |

An existing private key is never overwritten. If `~/.ssh/id_ed25519` already exists on a FIPS system, `ssh-setup` leaves it in place, and `ssh` may print a harmless warning that Ed25519 keys aren't allowed in FIPS mode. You can move that key aside to stop the warning.

SAS_lsm uses the key that `ssh-setup` installs automatically, because `id_ecdsa` and `id_rsa` are standard `ssh` keys.

### Manual setup

Use these steps where `ssh-copy-id` isn't available, as on some AIX and Solaris systems. In FIPS mode, use `-t ecdsa -b 384` instead of `-t ed25519`.

```sh
# on the controller, as the SAS installer account (once)
ssh-keygen -t ed25519 -N "" -f ~/.ssh/id_ed25519
cat ~/.ssh/id_ed25519.pub                              # copy this line

# on each tier host, as the tier user
mkdir -p ~/.ssh && chmod 700 ~/.ssh
echo "<paste the public key line>" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# back on the controller: must print the host name without asking for a password
ssh -o BatchMode=yes sas@meta1 hostname
```

### Connection reuse

With `settings.ssh_multiplex: true` (the default), SAS_lsm keeps one SSH connection open per host for 60 seconds, using OpenSSH `ControlMaster`. A run that makes many status checks therefore opens only a few connections. The control sockets live in `~/.ssh/sas_lsm-<hash>`.

Set `ssh_multiplex: false` if your home directory is on a filesystem that doesn't support UNIX sockets, such as some NFS mounts.

### Sites that don't allow SSH keys

- **Kerberos:** if your hosts already use Kerberos (GSSAPI) for SSH, add `-o GSSAPIAuthentication=yes` to `settings.ssh_options`. The account needs a valid ticket, for example from a keytab renewed by cron.
- **General rule:** SAS_lsm uses the system `ssh` client. Anything that works with `ssh -o BatchMode=yes user@host true` works with SAS_lsm, including host aliases, ports, `ProxyJump` and identity files set in `~/.ssh/config`.

### Host keys

The default `ssh_options` include `StrictHostKeyChecking=accept-new`. SAS_lsm records a new host's key on the first connection, and refuses to connect if that key later changes.

SAS_lsm 4.x used `StrictHostKeyChecking=no`, which silently accepted changed keys.

---

## 6. Configuration file reference

```yaml
version: 1          # required
settings: ...       # optional: global options
defaults: ...       # optional: values every tier inherits unless it overrides them
tiers: ...          # required: the ordered list of tiers
```

The file name without `.yaml` names the status directory. For example, `prod.yaml` writes to `<status_root>/prod/`. To manage several deployments from one controller, use one file per deployment.

### 6.1 `settings`

| Key | Default | Meaning |
|---|---|---|
| `status_root` | `/var/tmp/sas_lsm` | Where reports, the lock and collected logs are written. Avoid `/tmp`, which may be cleared at reboot. |
| `ssh_options` | `[-o, BatchMode=yes, -o, ConnectTimeout=10, -o, StrictHostKeyChecking=accept-new]` | Options passed to every `ssh` call. |
| `ssh_multiplex` | `true` | Reuse SSH connections (see [Connection reuse](#connection-reuse)). |
| `admin_email` | none | One address or a list. SAS_lsm mails the report after every start and stop, and after a failed status. |
| `mail_command` | `mailx` | Called as `<cmd> -s <subject> <addresses> < report`. |
| `archive_reports` | `true` | Keep a timestamped copy of every start/stop report in `<status dir>/archive/`. |
| `collect_logs` | `false` | When a tier fails, collect its recent logs and build a support bundle. Same as passing `--collect-logs` on every run. |
| `log_minutes` | `60` | Only collect logs changed within this many minutes. |
| `log_max_lines` | `5000` | Number of lines copied from the end of each log file. |

### 6.2 `defaults` and per-tier settings

You can set any key below under `defaults:` to apply it to every tier, or on an individual tier to override the default for **that tier only**. Unlike SAS_lsm 4.x, a value set on one tier never carries over to the next.

| Key | Default | Meaning |
|---|---|---|
| `user` | user running `sas_lsm` | Account that runs the tier's commands (the SAS installer account). |
| `configdir` | none | SAS configuration directory, such as `/opt/sas/config/Lev1`. Required for `sas-servers` and `metadata-cluster` tiers. |
| `parallel` | `false` | Run the tier's hosts at the same time instead of one after another. |
| `retries` | `10` | How many times to re-check status while waiting for UP or DOWN. |
| `retry_interval` | `6` | Seconds between those checks. |
| `tier_delay` | `15` | Seconds to pause after a tier is UP, before starting the next tier. |
| `command_timeout` | `900` | Seconds before a single command is cancelled. |
| `ok_exit_codes` | `[0, 1, 2, 4]` | Exit codes treated as success for start, stop and status commands. `sas.servers` can return a non-zero code even when it worked, so the tier's state is judged from its output (section 7.4). |

The longest SAS_lsm waits for a tier is roughly `retries × retry_interval` seconds, which is one minute with the defaults. Increase `retries` for slow tiers such as the middle tier (for example, `retries: 150` allows 15 minutes).

### 6.3 Tier keys

| Key | Applies to | Meaning |
|---|---|---|
| `name` | all (required) | Unique name. Used in messages and with `--from` / `--to`. |
| `type` | all | `sas-servers` (default), `metadata-cluster`, `lsf-grid` or `custom`. |
| `hosts` | all except `lsf-grid` | One or more host names. |
| `start`, `stop`, `status` | `custom` (required); others (optional) | The commands for this tier. Built-in types supply them, so set these only to override. |
| `restart` | optional | Command used when the first tier of a `start` is already running. If omitted, SAS_lsm stops the tier, waits for DOWN, then starts it. |
| `up_pattern`, `down_pattern` | `custom` (required); others (optional) | Regular expressions matched against status output ([7.4](#74-how-a-tier-is-judged-up-or-down)). |
| `logdir` | optional | Where to look for logs when collecting them. Defaults to `configdir`. |
| `userexits` | optional | Scripts run around actions ([section 8](#8-userexits)). |
| `userexit_dir` | optional | A SAS_lsm 4.x-style userexit directory ([8.3](#83-sas_lsm-4x-userexit-directories)). |
| `lsf` | `lsf-grid` (required) | LSF settings ([6.4.3](#643-lsf-grid)). |

**Placeholders in commands:**

- `{configdir}` and `{user}` are replaced when the configuration is loaded.
- `{host}` is replaced with each host name when the command runs.

### 6.4 Tier types

#### 6.4.1 `sas-servers`

Controls a host through its `sas.servers` script:

| Action | Command |
|---|---|
| start | `{configdir}/sas.servers start` |
| stop | `{configdir}/sas.servers stop` |
| status | `{configdir}/sas.servers status` |
| up / down patterns | `is UP` / `is NOT up` |

Put hosts that should start together in the same `sas-servers` tier. A single (non-clustered) metadata server is also a `sas-servers` tier, because the metadata host's `sas.servers` starts it.

#### 6.4.2 `metadata-cluster`

A SAS Metadata Server cluster with 3 or more nodes. This single tier replaces the 4.x `MDCN` / `MDCV` tier pairs.

```yaml
- name: metadata
  type: metadata-cluster
  hosts: [meta1, meta2, meta3]
```

- **start:** runs `MetadataServer.sh start` in the background on every node at the same time, then waits for quorum.
- **status:** the tier is UP when a majority of nodes are running, or when exactly half are running and one of them is the first node. If some nodes are down, the report adds a "degraded" note.
- **stop:** runs `MetadataServer.sh stopCluster` on the first running node. Any node still running after the wait is stopped individually with `MetadataServer.sh stop`.
- A node in the `PAUSED` state counts as not running.

Only one `metadata-cluster` tier is allowed per configuration.

#### 6.4.3 `lsf-grid`

Controls the Platform LSF daemons for SAS Grid Manager for Platform Suite. Instead of `hosts`, list the LSF hosts under `lsf:`.

```yaml
- name: lsf
  type: lsf-grid
  parallel: true
  lsf:
    master: gridctl                    # LSF master host (required)
    nodes: [node1, node2]              # other LSF hosts
    envdir: /opt/sas/lsf/conf          # LSF_ENVDIR, contains profile.lsf (required)
    serverdir: /opt/sas/lsf/10.1/linux2.6-glibc2.3-x86_64/etc   # LSF_SERVERDIR (required)
    pm_host: gridctl                   # Process Manager host (optional)
    pm_envdir: /opt/sas/pm/conf        # JS_ENVDIR, contains profile.js
    pm_serverdir: /opt/sas/pm/10.2/linux2.6-glibc2.3-x86_64/etc # JS_SERVERDIR
    ego_controls_daemons: false        # true if EGO starts res and sbatchd
    sudo: true                         # run LSF commands with sudo -n
```

**Start**
1. On the master: `lsadmin limstartup`, then `lsadmin resstartup`, then `badmin hstartup`.
2. The same three commands on each node. Nodes are started in parallel when `parallel: true`.
3. `jadmin start` on the Process Manager host.

With `ego_controls_daemons: true`, SAS_lsm starts only the LIM and lets EGO start the other daemons.

**Stop:** the reverse order. Process Manager first, then the nodes, then the master.

**Status:** a process check on every host.
- Master: lim, res, sbatchd, mbatchd, mbschd
- Nodes: lim, res, sbatchd
- Process Manager host: jfd

**Root access.** The LSF daemons run as root. With `sudo: true`, every LSF command runs as `sudo -n /bin/sh -c '. <envdir>/profile.lsf && <command>'`, so the tier user needs a passwordless sudoers rule. For example:

```
sas ALL=(root) NOPASSWD: /bin/sh -c . /opt/sas/lsf/conf/profile.lsf *, /bin/sh -c . /opt/sas/pm/conf/profile.js *
```

On older Linux releases, also add `Defaults:sas !requiretty`. Run `sas_lsm doctor` to confirm the sudo setup works.

**Scope.** This tier type controls only the LSF daemons. Put the SAS servers on the grid hosts (object spawner, Deployment Agent and so on) in a separate `sas-servers` tier after the `lsf-grid` tier; see `examples/cluster-grid.yaml`. Process Manager managed by EGO (`egosh service start ProcessManager`) is not built in. If you need it, use a `custom` tier or a userexit.

#### 6.4.4 `custom`

Controls anything that has start, stop and status commands, such as LASR servers or third-party services.

```yaml
- name: reporting-app
  type: custom
  hosts: [app01]
  start:  "/opt/app/bin/appctl start"
  stop:   "/opt/app/bin/appctl stop"
  status: "/opt/app/bin/appctl status"
  up_pattern: "is running"
  down_pattern: "is not running|is stopped"
```

Write commands exactly as you would type them in a shell on the tier host. You don't need the extra escaped quotes that SAS_lsm 4.x required.

If a start command leaves a process running in the background, redirect its output: `nohup cmd >/dev/null 2>&1 &`. Otherwise SAS_lsm waits up to 5 seconds for the output stream to close.

### 6.5 YAML rules used by SAS_lsm

SAS_lsm reads a small, strict subset of YAML. This lets it ship without third-party libraries.

- Indent with **spaces**, never tabs.
- Write lists as `[a, b, c]` or as indented `- item` lines.
- Quote any value that contains `: ` or ` #`, or that starts with a special character. Commands are safest in double quotes. Inside double quotes, write `\"` for a quote and `\\` for a backslash.
- `#` starts a comment, unless it is inside quotes.
- Anchors (`&` / `*`), tags (`!!`), `{ }` mappings and multi-line `|` / `>` values are rejected with a clear error.
- Unknown keys and misspellings are errors, reported with the line number.

---

## 7. Start, stop and status behaviour

### 7.1 `start [--from TIER]`

1. **Dependency check.** Every tier *before* TIER must be UP, and every tier *after* it must be DOWN. If any check fails, nothing is started and the exit code is 2. This prevents, for example, starting the middle tier while metadata is down, or starting on top of a half-running deployment.
2. **TIER itself.** If TIER is already running, SAS_lsm restarts it using `restart`, or stop, wait for DOWN, then start.
3. **Each tier in turn:**
   1. Run `pre_start` userexits.
   2. Start the tier.
   3. Wait for UP.
   4. Run `post_start_success` or `post_start_failure` userexits.
   5. Pause for `tier_delay`.

   If a tier doesn't reach UP, later tiers are **not** started and the exit code is 1.

`--from` defaults to the first tier. TIER can be a number or a name, for example `--from 3` or `--from midtier`.

### 7.2 `stop [--to TIER]`

Stops tiers from the last tier down to and including TIER, which defaults to the first tier. For each tier, SAS_lsm runs `pre_stop` userexits, stops the tier, waits for DOWN, then runs `post_stop_success` or `post_stop_failure` userexits.

If a tier does not stop, earlier tiers are left running, so metadata stays available while you investigate. The exit code is 1.

### 7.3 `status`

Checks every tier and prints each host's matching status lines. If a host's state is UNKNOWN, for example because it can't be reached, SAS_lsm retries up to `retries` times.

The exit code depends on the **last start or stop**, which is recorded in `<status dir>/.lastaction`:

| Last action | Deployment state | Exit code |
|---|---|---|
| start | all tiers UP | 0 |
| start | anything else | 1 (and mail, if configured) |
| stop | anything except UNKNOWN | 0 (a deliberate shutdown is not a failure) |
| any | any tier UNKNOWN | 4 |

Because of this, a `status` cron job that runs every few minutes alerts only when something is actually wrong.

### 7.4 How a tier is judged UP or DOWN

For each host, SAS_lsm matches the status command's output line by line against the up and down patterns:

| `up_pattern` matched | `down_pattern` matched | Host state |
|---|---|---|
| yes | no | UP |
| no | yes | DOWN |
| yes | yes | PARTIAL (some services up, some down) |
| no | no | UNKNOWN (unexpected output, or the host could not be reached) |

The tier's state comes from its hosts:

- **UP** when all hosts are UP.
- **DOWN** when all hosts are DOWN.
- **UNKNOWN** if any host is UNKNOWN.
- **PARTIAL** for any other mix.

Metadata clusters use the quorum rule from section 6.4.2 instead.

Patterns are Go regular expressions, which are very close to `egrep` syntax. They are case-sensitive, so `is UP` does not match `is NOT up`.

---

## 8. Userexits

Userexits are your own scripts, run around tier actions. Typical uses are notifying users, draining grid jobs and starting the Deployment Agent.

### 8.1 Events

| Event | When it runs |
|---|---|
| `pre_start` | before the tier is started |
| `post_start_success` / `post_start_failure` | after the tier reached, or failed to reach, UP |
| `pre_stop` | before the tier is stopped |
| `post_stop_success` / `post_stop_failure` | after the tier reached, or failed to reach, DOWN |
| `pre_status` / `post_status` | around the tier's status check |

### 8.2 Listing userexits in the configuration

```yaml
- name: compute
  hosts: [comp1, comp2]
  userexits:
    pre_stop:
      - /opt/sas/utilities/userexits/notify_users.sh          # short form
      - run: /opt/sas/utilities/userexits/drain_jobs.sh {host}
        required: true                                         # failure stops the run
        on: all                                                # every host (default: first host only)
```

How userexits run:

- Scripts run on the tier host, as the tier user, in the order listed.
- Exit code 0 means success.
- If a **required** script fails:
  - for a `pre_*` event, the run stops before the action;
  - for a `post_*` event, the tier is marked failed.
- If any other script fails, SAS_lsm reports a warning and continues.

To skip all userexits for one run, use `--no-userexits`.

### 8.3 SAS_lsm 4.x userexit directories

Setting `userexit_dir: /opt/sas/userexits/compute` keeps the 4.x file-naming convention working:

- File names follow `NN_<pre|post>_<start|stop|status>[_<success|failure>]_name`.
- `_required_` anywhere in the name marks the script as required.

These scripts run after any scripts listed in the configuration, sorted by path. Two 4.x problems are fixed:

- Names match **in any case**. The 4.x guide showed upper-case names, which 4.x never actually matched.
- A `post` script without `_success_` or `_failure_` now runs after **either** result, as the 4.x guide described.

### 8.4 Testing userexits

This replaces `userExit_tester.bash`. It uses the same matching code as a real run and reads the directory on the tier host itself.

```sh
sas_lsm userexits -c prod.yaml --tier compute
```

---

## 9. Running SAS_lsm

### 9.1 Commands

```
sas_lsm start     [--from TIER] [--dry-run] [--collect-logs] [--no-userexits] [-q] -c FILE
sas_lsm stop      [--to TIER]   [--dry-run] [--collect-logs] [--no-userexits] [-q] -c FILE
sas_lsm status    [--collect-logs] [--no-userexits] [-q] -c FILE
sas_lsm validate  -c FILE
sas_lsm doctor    -c FILE
sas_lsm ssh-setup --hosts H1,USER@H2,... [--user USER] [--key-type T] [--yes] [--verbose]
sas_lsm ssh-setup -c FILE [--key-type T] [--yes] [--verbose]
sas_lsm userexits [--tier TIER] -c FILE
sas_lsm discover  --hosts h1,h2,... --configdir DIR [--user USER]
sas_lsm convert   OLD.cfg
sas_lsm version   [--check]
```

If `-c` is omitted, SAS_lsm uses `$SAS_LSM_CONFIG`, then `./sas_lsm.yaml`.

**4.x options still work**, so existing cron entries and scripts don't break:

| 4.x option | Equivalent |
|---|---|
| `-a N` | `start --from N` |
| `-o N` | `stop --to N` |
| `-s` | `status` |
| `-e` | `--collect-logs` |
| `-x` | `--no-userexits` |
| `-v` | `version` |

### 9.2 Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | An action failed, `status` after a start found tiers not fully UP, or `ssh-setup` left a host not ready |
| 2 | `start` refused: dependency check failed (nothing was changed) |
| 3 | Configuration or usage error |
| 4 | State UNKNOWN (a host was unreachable or returned unexpected output) |
| 5 | Another SAS_lsm run is active for this configuration |

### 9.3 Files written

All files are under `<status_root>/<config name>/`:

| File | Content |
|---|---|
| `report.start`, `report.stop`, `report.status` | Full report of the latest run, including every command and its raw output. The console shows a summary. |
| `archive/<timestamp>.report.<action>` | History of start and stop reports. |
| `.lastaction` | The last start or stop and its result. Determines `status` exit codes. |
| `.lock/` | Present while a run is active. A lock left by a killed run is removed automatically. |
| `logfiles/`, `logfiles.ANALYSIS` | Logs collected after a failure, plus a summary of the most frequent `ERROR` lines. |
| `support-bundle-<timestamp>.tar.gz` | Reports, collected logs and analysis, ready to attach to a SAS Technical Support case. |

### 9.4 Log collection

Log collection happens only when a tier fails **and** either `--collect-logs` or `collect_logs: true` is set. For each host in the failed tier, SAS_lsm:

1. Finds `*.log` files under `logdir` changed in the last `log_minutes` minutes (at most 50 per host).
2. Copies the last `log_max_lines` lines of each file through the existing SSH connection. It doesn't use `scp` or extra disk-space checks.
3. Writes `logfiles.ANALYSIS`, prints the 10 most frequent errors, and builds the support bundle.

### 9.5 Scheduling with cron

```cron
# status every 10 minutes; mail is sent only when something is wrong
*/10 * * * *  /opt/sas/utilities/sas_lsm/sas_lsm status -q -c /opt/sas/utilities/sas_lsm/prod.yaml
# weekly maintenance restart
0 2 * * 0     /opt/sas/utilities/sas_lsm/sas_lsm stop -q -c /opt/sas/utilities/sas_lsm/prod.yaml && /opt/sas/utilities/sas_lsm/sas_lsm start -q -c /opt/sas/utilities/sas_lsm/prod.yaml
```

`-q` turns off console output; the report file is still written. Runs can't overlap: a second run exits with code 5.

### 9.6 Start at boot with systemd (Linux)

Create `/etc/systemd/system/sas-lsm.service` on the controller:

```ini
[Unit]
Description=SAS 9.4 deployment (SAS_lsm)
After=network-online.target sshd.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
User=sas
Environment=SAS_LSM_CONFIG=/opt/sas/utilities/sas_lsm/prod.yaml
ExecStart=/opt/sas/utilities/sas_lsm/sas_lsm start -q
ExecStop=/opt/sas/utilities/sas_lsm/sas_lsm stop -q
TimeoutStartSec=0
TimeoutStopSec=1800

[Install]
WantedBy=multi-user.target
```

Then enable it:

```sh
systemctl daemon-reload && systemctl enable sas-lsm
```

In a multi-host deployment, the other hosts must be running and reachable over SSH before the controller's `start` runs. Otherwise the dependency check finds UNKNOWN tiers and nothing starts. Either make the controller the last host to boot, or schedule the start for some time after boot.

---

## 10. Migrating from SAS_lsm 4.x

1. Convert the configuration. Any items that need review are printed to the terminal.

   ```sh
   sas_lsm convert /path/to/SAS_lsm.cfg > prod.yaml
   ```

   The 4.x file is a bash script, so `convert` reads it with `bash` exactly as 4.x did. Only convert files you trust.
2. Run `sas_lsm validate`, then `sas_lsm doctor`, then `sas_lsm start --dry-run`. Compare the plan with how 4.x behaved.
3. Run `sas_lsm status` and compare the result with `SAS_lsm.sh -s`.
4. Point your cron entries and boot scripts at the new binary. The 4.x options are still accepted, but `-c` must now point to the YAML file.

Your existing SSH keys keep working. If the controller is now in FIPS mode and your 4.x setup used an Ed25519 key, run `sas_lsm ssh-setup -c prod.yaml` to install an ECDSA key alongside it.

### What `convert` does

| SAS_lsm 4.x | SAS_lsm 5 |
|---|---|
| `MAXTIERS` and numbered `TIER*[n]` arrays | An ordered `tiers:` list. The count is automatic. |
| Three `MDCN` tiers plus one `MDCV` tier | One `metadata-cluster` tier |
| `TIERSTAR` = `<dir>/sas.servers start` | `type: sas-servers` with `configdir: <dir>` |
| Any other tier | `type: custom` with its commands; outer quotes are removed |
| `TIERSTOS` / `TIERSTAS` | `up_pattern` / `down_pattern` |
| `TIERNAME` | A name that's easy to type (`Mid Tier` → `mid-tier`) |
| `INSTID`, `CONFIGDIR` | `defaults.user`, `defaults.configdir`, overridden per tier where they differed |
| `MAXRETRY`, `SLEEP` | `defaults.retries`, `defaults.retry_interval` |
| `STATUSROOT`, `SASADMIN`, `SSHOPTIONS` | `settings.status_root`, `settings.admin_email`, `settings.ssh_options` |
| `StrictHostKeyChecking=no` | Replaced by `accept-new` |
| `TIEREXIT` | `userexit_dir` |
| `TIERREST` | Not needed. Restart is stop, wait for DOWN, then start. |

### Behaviour changes to be aware of

| Area | 4.x | 5 |
|---|---|---|
| SSH direction | Documented as multi-directional | Controller to tier hosts only |
| SSH setup | Manual | `ssh-setup`, FIPS-aware, one password per host |
| Tier settings | A `CONFIGDIR` / `INSTID` value carried into later tiers | Each tier inherits only from `defaults:` |
| First tier of `start` | Always restarted | Restarted only if it isn't already DOWN |
| Hosts per tier | One | Any number, optionally in parallel |
| Required userexit success | Exit codes 0, 1, 2, 4 | Exit code 0 only |
| Userexit file names | Lower case only; `post` scripts needed `_success_` / `_failure_` | Any case; `post` scripts without a result run after either result |
| Version check | Contacted GitHub on every run, and could hang without internet access | Only with `sas_lsm version --check`, with a 5-second timeout |
| Pause between tiers | Fixed 15 seconds | `tier_delay`, configurable per tier |
| Concurrent runs | Detected with `ps \| grep` | Lock directory, with stale-lock cleanup |
| Log collection | `scp` of whole files | Last `log_max_lines` lines of recent logs, plus a support bundle |
| Exit codes | 0 and 1 | 0–5 (see [9.2](#92-exit-codes)) |

---

## 11. Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `ssh-setup` lists a host as `[needs key]` | Expected the first time. Continue, and type that account's password when prompted. |
| `ssh-setup` lists a host as `[problem]` | The host name can't be resolved, or the host or its `sshd` can't be reached. Fix the name or network, then run `ssh-setup` again. Use `--verbose` to see ssh's own message. |
| `ssh-setup` lists a host as `[host key]` | The host's key changed since it was recorded. Confirm the change is legitimate, run `ssh-keygen -R <host>`, then run `ssh-setup` again. |
| `ssh-setup` summary shows a host as `failed` | The password was wrong, or the account can't log in with a password. If the key was copied but login still fails, check that `~/.ssh` on that host is mode `700`, `authorized_keys` is mode `600`, and the home directory isn't group-writable. |
| `Ed25519 keys are not allowed in FIPS mode` | The controller is in FIPS mode and `~/.ssh/id_ed25519` exists. Run `sas_lsm ssh-setup` to install an ECDSA key. The warning is harmless afterwards; move `id_ed25519` aside to stop it. |
| `ssh to sas@host failed (exit 255): Permission denied (publickey...)` during start/stop/status | The controller's key isn't installed for that account. Run `sas_lsm ssh-setup -c <config>`. |
| A tier is UNKNOWN but the host is reachable | The status output matched neither pattern. Check the raw output in `report.status`, which records every command's output, and adjust `up_pattern` / `down_pattern`. |
| `sas.servers` works interactively but not from SAS_lsm | Non-interactive SSH sessions don't read some profile files. Run `ssh -o BatchMode=yes sas@host '<configdir>/sas.servers status'` to reproduce, then fix the environment in `~/.bashrc` or in the script. |
| Start times out on the middle tier | Raise `retries` for that tier, since web application servers can take 10–20 minutes. If `sas.servers start` itself runs longer than 15 minutes, also raise `command_timeout`. |
| `another SAS_lsm run (pid N) is active` | A run is already in progress, possibly from cron. If pid N is not SAS_lsm, delete `<status dir>/.lock`. |
| `lsf-grid` start or stop does nothing | Run `sas_lsm doctor`. The `passwordless sudo` check must pass; you may also need to disable `requiretty`. |
| A command hangs until `command_timeout` | A background process is holding the session open. Add `>/dev/null 2>&1 &` to background start commands in custom tiers. |
| `ControlPath too long` or socket errors | Set `settings.ssh_multiplex: false`. |
| `tab characters are not allowed` | Replace tabs with spaces in the configuration file. |

For problems with SAS_lsm itself, open an issue on GitHub. Attach `<status dir>/support-bundle-*.tar.gz` or the report file, after removing anything confidential.
