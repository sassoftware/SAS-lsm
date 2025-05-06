# SAS_lsm

## Overview

The  SAS_lsm utility provides consistent management of single- or multi-tiered SAS-related services for UNIX/Linux deployments from a single shell script. Consistent management includes tier dependency checking, starting, stopping, status checking (tier- and deployment-based), error log collection and analysis. You can execute the script from the operating system command line, via  a scheduled cron process, or via your operating system reboot facility.

### What's New

SAS_lsm 4.1 adds the following features and improvements:

* UserExit Management - Add capabilities to allow UserExit scripts to be called pre- or post- STATUS checks on a tier. 
* Platform LSF GRID Support - Deliver utility script to easily enable integration with Platform LSF GRID deployments.
* Configuration File Wizard - Logic updates to check and catch additional user-errors when running the Wizard.
* Documentation - Denote considerations when running SAS_lsm regularly via CRON or schedulers.

### Prerequisites

* Passwordless-SSH must be enabled (into the SAS Installation  User account on each machine of the deployment). This should be multi-directional to ensure full functionality.
* The BASH and SH shells are supported. Use of BASH is highly recommended.

### Installation

Refer to detailed installation documentation provided in package as **Install-Update-Guide.pdf**. 

### Running

The usage function inside SAS_lsm describes how to use the utility. A basic version of this is copied below.

`[bash | sh] ${PROG} [-a <NUM> | -o <NUM> | -s] -c <CFG> [-e]`

* **-a NUM**     start deployment tier services from tier NUM to MAXTIERS
* **-o NUM**     stop deployment tier services from MAXTIERS to tier NUM
* **-s**         provide status of all deployment tier services
* -**c CFG**     specify configuration file
* **-e**         [OPTIONAL] extract potential tier error logs to a centralized location under !STATUSROOT/!CFG/

*NOTE: Only one action option (-a, -o, and -s) can be run in a single command.*


### Examples

*Start services from, above, and including tier 1:* `bash SAS_lsm -a 1 -c /path/to/config-file.cfg`

*Stop services down to (and including) tier 4. Perform log analysis if errors are encountered:* `bash SAS_lsm -o 4 -c /path/to/config-file.cfg -e`

*Check tier status for all tiers in configuration file:* `bash SAS_lsm -s -c /path/to/config-file.cfg`

## Contributing

We welcome your contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details on how to submit contributions to this project.

* Please contribute changes on a function-based level. Update or add functions as found in the repository's "lsm_components" subdirectory. Use the make_lsm.bash script (in the repository root) to combine all function files to a master SAS_lsm script before pushing changes.
* This project is looking for a new primary maintainer!

## License

This project is licensed under the [Apache 2.0 License](LICENSE).

## Additional Resources

* [SAS Note 58231](http://support.sas.com/kb/58/231.html)
* [SAS_lsm Demo Blog](https://communities.sas.com/t5/SAS-Communities-Library/The-SAS-lsm-Utility-Makes-it-Easy-to-Control-SAS-Servers-in-a/ta-p/418165)
* [SGF 2017 Proceedings](http://support.sas.com/resources/papers/automating-management-unix-linux-multi-tiered-sas-services.pdf)
* [SGF 2018 Proceedings](https://www.sas.com/content/dam/SAS/support/en/sas-global-forum-proceedings/2018/1921-2018.pdf)
