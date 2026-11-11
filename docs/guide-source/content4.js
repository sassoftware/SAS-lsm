const { H1, H2, P, Code, Tbl } = require('./lib');

module.exports = () => [
  H1('Appendix: Example Configuration Files'),
  H2('Single Machine'),
  P('Everything runs on one machine under the account that runs SAS_lsm, so no SSH setup is needed.'),
  Code(
`version: 1

defaults:
  user: sasinst
  configdir: /opt/sas/config/Lev1

tiers:
  - name: sas
    hosts: [localhost]`),

  H2('Metadata, Compute, and Middle Tier'),
  Code(
`version: 1

settings:
  status_root: /var/tmp/sas_lsm
  admin_email: [sasadmin@example.com]
  collect_logs: true

defaults:
  user: sasinst
  configdir: /opt/sas/config/Lev1

tiers:
  - name: metadata
    hosts: [sasmeta01.example.com]

  - name: compute
    hosts: [sascomp01.example.com, sascomp02.example.com]
    parallel: true
    userexits:
      pre_stop:
        - /opt/sas/utilities/userexits/notify_users.sh

  - name: midtier
    hosts: [sasmid01.example.com]
    configdir: /opt/sas/configmid/Lev1
    retries: 150
    command_timeout: 1800`),

  H2('Metadata Cluster, Platform LSF Grid, and Middle Tier'),
  Code(
`version: 1

settings:
  status_root: /var/tmp/sas_lsm
  collect_logs: true

defaults:
  user: sasinst
  configdir: /opt/sas/config/Lev1

tiers:
  - name: metadata
    type: metadata-cluster
    hosts: [meta1.example.com, meta2.example.com, meta3.example.com]

  - name: lsf
    type: lsf-grid
    parallel: true
    lsf:
      master: gridctl.example.com
      nodes: [node1.example.com, node2.example.com, node3.example.com]
      envdir: /opt/sas/lsf/conf
      serverdir: /opt/sas/lsf/10.1/linux2.6-glibc2.3-x86_64/etc
      pm_host: gridctl.example.com
      pm_envdir: /opt/sas/pm/conf
      pm_serverdir: /opt/sas/pm/10.2/linux2.6-glibc2.3-x86_64/etc

  - name: grid-compute
    hosts: [gridctl.example.com, node1.example.com, node2.example.com, node3.example.com]
    parallel: true
    userexits:
      post_start_success:
        - run: /opt/sas/utilities/userexits/start_deployment_agent.sh
          on: all
      pre_stop:
        - run: /opt/sas/utilities/userexits/drain_grid_jobs.sh
          required: true

  - name: midtier
    hosts: [mid1.example.com]
    retries: 150
    command_timeout: 1800`),

  H1('Appendix: Command Reference'),
  Tbl([
    ['Command', 'Purpose'],
    ['sas_lsm start [--from TIER] [--dry-run] [--collect-logs] [--no-userexits] [-q] -c FILE', 'Start tiers'],
    ['sas_lsm stop [--to TIER] [--dry-run] [--collect-logs] [--no-userexits] [-q] -c FILE', 'Stop tiers'],
    ['sas_lsm status [--collect-logs] [--no-userexits] [-q] -c FILE', 'Check all tiers'],
    ['sas_lsm validate -c FILE', 'Check the configuration file'],
    ['sas_lsm doctor -c FILE', 'Check SSH, accounts, paths, and permissions on every host'],
    ['sas_lsm ssh-setup --hosts H1,USER@H2,... [--user USER] [--key-type TYPE] [--yes] [--verbose]', 'Create and install SSH keys on a list of hosts (no configuration file needed)'],
    ['sas_lsm ssh-setup -c FILE [--key-type TYPE] [--yes] [--verbose]', 'Create and install SSH keys on every host in a configuration file'],
    ['sas_lsm userexits [--tier TIER] -c FILE', 'List the UserExit scripts that will run'],
    ['sas_lsm discover --hosts H1,H2,... --configdir DIR [--user USER]', 'Draft a configuration file from your machines'],
    ['sas_lsm convert OLD.cfg', 'Convert a SAS_lsm 4.x configuration file'],
    ['sas_lsm version [--check]', 'Show the version (--check also shows the newest release on GitHub)'],
    ['sas_lsm help', 'Show command help'],
  ], [6000, 3360], { mono: [0] }),
];
