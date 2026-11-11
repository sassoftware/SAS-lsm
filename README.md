## SAS_lsm

### Overview

SAS_lsm provides consistent start, stop and status management for single-tier and multi-tier SAS 9.4 deployments on UNIX/Linux. It runs from one command on one machine.

### What's New

Version 5 is a modernization of SAS-lsm.
The code of SAS-lsm version 4.1.0 was rewritten in Go using automated tools. The core features of SAS-lsm version 4.1.0 are all supported in SAS-lsm version 5.0.0.
The user configuration was simplified and utilities were added to make initial setup easier.
The configuration file was updated to use a standard YAML syntax instead of a custom-defined format. Converter functions have been made available to permit migration from existing systems where SAS-lsm version 4.x is in use.

Notable updates with SAS-lsm version 5.0.0:

- **One file to install.** A single static binary with no runtime dependencies. Nothing is installed on the SAS hosts.
- **YAML configuration** with built-in tier types: `sas-servers`, `metadata-cluster`, `lsf-grid` and `custom`.
- **Setup helpers:** `ssh-setup` installs SSH keys (FIPS-aware, works before a configuration exists), `discover` drafts a configuration, `convert` imports a 4.x configuration, and `doctor` checks every host before the first run.
- **SSH only from the controller** to each tier host, using the system OpenSSH client. Connections are reused, so a run opens few of them. Bidirectional SSH is no longer required.
- **Multiple hosts per tier,** started in parallel when you choose, plus metadata cluster quorum handling and LSF grid control. Note that for stability of the SAS 9 platform, parallel statup is not recommended in most setups. Administrators can test parallel startup if users desire startup speed improvements.
- **Safe operation:** `--dry-run` plans, a lock that blocks overlapping runs, clear exit codes, and a support bundle on failure.
- **Legacy compatible:** Provides support for converting SAS_lsm version 4.x configuration files, handles 4.x command syntax, related support for drop-in update of an existing SAS_lsm version 4.x implementation.

### Prerequisites / System Requirements

* Key-based SSH authentication (also known as "passwordless SSH") is required. SSH used to communicate from the controller (where you run SAS_lsm) to other nodes. The system-default SSH libraries are used. SAS_lsm can help set up the SSH keys between your machines, if this is not already configured.
* Go (version 1.21+) is required to run SAS_lsm.
* The BASH shell is required for one-time use when converting a legacy SAS_lsm configuration file.

### Quick Start

```sh
sas_lsm ssh-setup --hosts meta1.sas.com,comp1.sas.com,mid1.sas.com --user sas          # install SSH keys / set up Passwordless-SSH (one password per host)
sas_lsm discover  --hosts meta1.sas.com,comp1.sas.com,mid1.sas.com --configdir /opt/sas/config/Lev1 --user sas > prod.yaml           # discover existing SAS services on provided hosts and generate a configuration file "prod.yaml"
                                                                                                                                     # or: sas_lsm convert SAS_lsm.cfg > prod.yaml (if you have an existing SAS-lsm version 4.x configuration file)
sas_lsm validate  -c prod.yaml           # validates the configuration file
sas_lsm doctor    -c prod.yaml           # inspects SSH capabilities and verifies environment of each host should be able to run SAS-lsm
sas_lsm start     -c prod.yaml --dry-run           # shows steps that would be performed in a start operation (from tier 1, as --from N was not specified)
sas_lsm stop      -c prod.yaml --dry-run           # shows steps that would be performed in a stop operation (to tier 1, as --to N was not specified)
sas_lsm status    -c prod.yaml           # runs a status check, which is a way to actually test the setup across all machines in the configuration file "prod.yaml" without making any change to service status
sas_lsm start     -c prod.yaml           # actual run of SAS_lsm, this would start services beginning on tier 1
```

### Installation

Download and build from source or use the prebuilt binary in **[bin/sas_lsm](bin/sas_lsm)**

Refer to the Quick Start section above for basic setup and testing. 

Sample configuration files are in [examples/](examples/).
The complete installation and usage guide is in **[docs/SETUP.md](docs/SETUP.md)**.
Further, detailed documentation is available in **[docs/SAS_lsm5-Install-Usage-Guide.pdf](docs/SAS_lsm5-Install-Usage-Guide.pdf)**

### Usage

`sas-lsm start -c <configfile.yaml>`      Start all tiers
`sas-lsm stop -c <configfile.yaml>`       Stop all tiers
`sas-lsm status -c <configfile.yaml>`     Show the current state of services (on all tiers)

Run the command `sas_lsm --help` for additional usage examples.



### Contributing

We welcome your contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) and [ContributorAgreement.txt](ContributorAgreement.txt) for details on how to submit contributions to this project.
Commits must include `Signed-off-by:`. Contributions created with AI assistance must also include `Assisted-by:` or `Generated-by:`.

### Building project from repository

```sh
make build    # build/sas_lsm
make test     # go vet and unit tests
make e2e      # end-to-end tests against simulated SAS hosts (no real SSH needed)
make dist     # Linux amd64/arm64/ppc64le, AIX ppc64, Solaris amd64
```

Requires Go 1.21 or later.
The code uses only the standard Go library.

### Repository Layout

| Path | Purpose |
|---|---|
| `bin/sas_lsm` | Precompiled binary, use this to actually run sas_lsm commands |
| `cmd/sas_lsm` | Command-line entry point, including the 4.x option translation |
| `internal/config` | Loading and validation of the YAML configuration, and the built-in tier types |
| `internal/yamlite` | A small parser for the subset of YAML used here |
| `internal/runner` | Running commands locally or through OpenSSH |
| `internal/engine` | Start/stop/status logic, quorum and LSF handling, userexits, log collection, reports |
| `internal/tools` | The `doctor`, `ssh-setup`, `convert` and `discover` commands |
| `test/e2e.sh` | End-to-end tests using fake `ssh`, `ssh-copy-id`, `ssh-keygen` and simulated SAS scripts |



### License

This project is licensed under the [Apache 2.0 License](LICENSE).
