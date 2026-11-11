const { H1, H2, H3, P, Note, Important, Bullet, Numbered, Code, Tbl, Gap } = require('./lib');

module.exports = () => [
  // ------------------------------------------------------------ INTRODUCTION
  H1('Introduction'),
  P('Have you ever experienced any of the following issues?'),
  Bullet('You try to restart SAS® servers to avoid loss of productivity, but the restart fails, causing delays.'),
  Bullet('You struggle to find and analyze the right SAS logs when something goes wrong.'),
  P('Or maybe you want other processes to help you, such as these:', { spacing: { before: 120, after: 140 } }),
  Bullet('You want to run a process before or after you start or stop SAS services.'),
  Bullet('You want SAS to notify you when something in the system stops working.'),
  P('SAS Technical Support developed the SAS Local Services Management (SAS_lsm) utility to address these problems. SAS_lsm starts, stops, and checks the status of SAS services across every tier of a SAS 9.4 deployment on UNIX or Linux, in the correct order, from a single command on a single machine. It is available via SAS Note 58231, "Utility that manages single- or multi-tiered SAS® related services for UNIX and Linux deployments."', { spacing: { before: 120, after: 140 } }),
  P('**SAS_lsm 5** is a rewrite of the utility. It keeps the ordering and dependency checks that are the core of SAS_lsm, and changes how the utility is installed and configured:'),
  Bullet('**One file to install.** SAS_lsm is now a single executable program. Nothing needs to be installed on the other machines in your deployment.'),
  Bullet('**A simpler configuration file.** Deployments are described in a YAML file. Built-in *tier types* supply the commands and status checks for SAS servers, so most tiers need only a name and a list of host names.'),
  Bullet('**Setup helpers.** New commands install SSH keys on every host with one password per host (`ssh-setup`, which also supports FIPS-enabled systems), draft a configuration file for you (`discover`), convert a SAS_lsm 4.x configuration file (`convert`), and check every host before the first run (`doctor`).'),
  Bullet('**SSH in one direction only.** Passwordless SSH is needed only from the SAS_lsm controller to each host, not between all machines.'),
  Bullet('**Several hosts per tier**, optionally controlled in parallel, plus built-in support for clustered SAS® Metadata Servers and Platform LSF grids.'),
  Bullet('**Safer operation**: a dry-run mode that shows what would happen, protection against two runs at once, detailed exit codes, and a support bundle for SAS Technical Support when a tier fails.'),
  P('Existing capabilities are retained: metadata horizontal clustering, control of SAS® LASR™ Analytic Server and third-party services, start-from and stop-at tier maintenance actions, centralized log collection and analysis, the UserExit framework, and e-mail notification.', { spacing: { before: 120, after: 140 } }),
  Note('If you are upgrading from SAS_lsm 4.x, read this guide through "Deploying SAS_lsm," and then see "Migrating from SAS_lsm 4.x." Your existing configuration file can be converted automatically.'),

  // ------------------------------------------------------------ ARCHITECTURE
  H1('Understanding the SAS_lsm System Architecture'),
  P('SAS_lsm runs from a single machine (the *SAS_lsm controller*), and it runs commands on the other machines that host SAS services by using the secure shell (SSH) protocol. Passwordless SSH is required from the SAS_lsm controller to each machine, for the account that starts and stops SAS services on that machine.'),
  P('You define machine names, tiers, and other settings in a SAS_lsm configuration file. A SAS_lsm controller can store any number of configuration files, so you can manage many SAS environments from one machine. SAS_lsm reads the configuration file that you specify and controls each tier in the proper order.'),
  P('The SAS_lsm controller might be, but does not have to be, one of the machines in your SAS deployment. Many sites run SAS_lsm on their SAS Metadata Server host, because that machine is started first. When a tier runs on the controller itself under the same account that runs SAS_lsm, SAS_lsm runs the commands directly instead of through SSH. Other sites prefer a dedicated management host that is separate from SAS. Both methods work equally well.'),
  Code(
`                       ssh (one direction only)
  SAS_lsm controller  ──────────────────────────►  meta1, comp1, comp2, mid1 ...
  (sas_lsm + YAML)      as the SAS installer account   (runs sas.servers, etc.)`),

  H2('Secure Shell (SSH) Considerations'),
  P('SAS_lsm requires passwordless SSH from the SAS_lsm controller to every host where it runs commands. A single SAS_lsm action can run many commands on each host, so entering a password each time is not practical. SAS_lsm 5 includes a command, `sas_lsm ssh-setup`, that creates an SSH key and installs it on every host you list. You enter each account\'s password once. See "Set Up Passwordless SSH."'),
  P('SSH is needed in **one direction only**: from the controller to each tier host. Tier hosts never connect to each other or back to the controller. This applies to Platform LSF grid tiers too, because the controller controls every grid node directly.'),
  P('**FIPS-enabled systems.** If the SAS_lsm controller runs in FIPS mode, Ed25519 SSH keys cannot be used. The ssh-setup command detects FIPS mode automatically and uses an ECDSA or RSA key instead.'),
  P('SSH behaves differently when commands are run in batch mode than in an interactive session. In batch mode, the system profile (for example, **/etc/profile**) is often not read. SAS_lsm uses batch mode only. If your SAS environment depends on variables set in a system profile, make sure that they are also set in the SAS installer account\'s own shell profile (for example, **${HOME}/.bashrc**). The `sas_lsm doctor` command helps you find this kind of problem.'),
  P('SAS_lsm uses the operating system\'s own `ssh` command. Anything that works with `ssh -o BatchMode=yes user@host` also works with SAS_lsm, including host aliases, non-standard ports, and settings in **${HOME}/.ssh/config**. Sites that use Kerberos for SSH can use it instead of SSH keys.'),
  P('The `ssh_options` setting in the configuration file lists options that SAS_lsm adds to every SSH command. For most sites, the default does not need to be changed. The default accepts the host key of a new host the first time SAS_lsm connects to it, and refuses to connect if that key changes later.'),

  H2('Additional Considerations'),
  P('SAS recommends that you start and stop services with the SAS installer account that you used when you originally deployed SAS. Problems can occur if the account that starts SAS services does not have permission to the files. Files are owned by the SAS installer account, so using that account avoids such problems. SAS_lsm does not run as **root**.'),
  Note('You can determine the SAS installer account by checking the ownership of a deployed SAS file such as *SAS-configuration-directory*/Lev*N*/sas.servers.'),

  H2('SAS_lsm Tiers'),
  P('A *tier* is a group of one or more machines whose services are started together. SAS_lsm starts tiers in the order in which they are listed in the configuration file and stops them in reverse order. A typical deployment has a metadata tier, a compute tier, and a middle tier, in that order.'),
  P('Before it starts a tier, SAS_lsm checks that the tiers before it are running and the tiers after it are stopped. After it starts a tier, SAS_lsm waits until every host in the tier reports that its services are running before it starts the next tier.'),

  H2('SAS_lsm Tier Types'),
  P('Each tier has a *type*. The type tells SAS_lsm which commands start, stop, and check the tier, and how to read the status output. SAS_lsm 5 has four tier types:'),
  Tbl([
    ['Tier Type', 'Use For', 'What You Must Provide'],
    ['sas-servers\n*(default)', 'Any machine whose SAS services are controlled by the sas.servers script: metadata (single server), compute, and middle-tier machines.', 'Host names and the SAS configuration directory'],
    ['metadata-cluster', 'A clustered SAS Metadata Server with three or more nodes. Replaces the MDCN and MDCV tier types of SAS_lsm 4.x.', 'The host names of all cluster nodes and the SAS configuration directory'],
    ['lsf-grid', 'The Platform LSF daemons of SAS® Grid Manager for Platform Suite.', 'The LSF master and node host names, and the LSF directories'],
    ['custom', 'Anything else that has start, stop, and status commands: SAS LASR Analytic Server, third-party services, and so on.', 'The commands and the text that status output contains when the service is up or down'],
  ], [2000, 4360, 3000], { mono: [0] }),
  Gap(),
  P('**sas-servers.** This is the tier type for most machines. SAS_lsm runs **sas.servers start**, **sas.servers stop**, and **sas.servers status** from the configuration directory that you set for the tier. A host is considered up when the status output contains **is UP** and no service reports **is NOT up**. A single (non-clustered) metadata server is a **sas-servers** tier.'),
  P('**metadata-cluster.** SAS_lsm starts the metadata server on all nodes at the same time, by using **MetadataServer.sh**, and then waits until the cluster has a quorum. The cluster is considered up when a majority of nodes are running (or exactly half, including the first node listed). To stop the cluster, SAS_lsm runs **MetadataServer.sh stopCluster** on a running node, and then stops any node that is still running. You no longer need separate MDCN and MDCV tiers: one **metadata-cluster** tier replaces them. Only the metadata server process is controlled by this tier. If the metadata cluster nodes also run other SAS services, add a **sas-servers** tier for those machines later in the list.'),
  P('**lsf-grid.** SAS_lsm starts and stops the Platform LSF daemons on the LSF master and each grid node, and optionally Process Manager. This replaces the **SAS_lsm_Grid_TEMPLATE.sh** UserExit script from SAS_lsm 4.x. See "Enable Control of a Platform LSF Grid."'),
  P('**custom.** You supply the commands and the text that identifies a running or stopped service. See "Enable Control of Third-Party Services" and "Enable Control of SAS LASR Analytic Server."'),

  // ------------------------------------------------------------ CONFIG FILE OVERVIEW
  H2('The SAS_lsm Configuration File'),
  P('The configuration file is a text file in YAML format, with four sections:'),
  Tbl([
    ['Section', 'Required?', 'Contents'],
    ['version', 'Yes', 'Always 1.'],
    ['settings', 'No', 'Settings for SAS_lsm itself, such as where reports are written and who receives e-mail.'],
    ['defaults', 'No', 'Values that every tier uses unless the tier sets its own value, such as the SAS installer account and the SAS configuration directory.'],
    ['tiers', 'Yes', 'The tiers of the deployment, in start order.'],
  ], [1800, 1400, 6160], { mono: [0] }),
  Gap(),
  P('Here is a complete configuration file for a typical deployment with a metadata server, two compute machines, and a middle-tier machine:'),
  Code(
`version: 1

settings:
  status_root: /var/tmp/sas_lsm
  admin_email: [sasadmin@example.com]

defaults:
  user: sasinst
  configdir: /opt/sas/config/Lev1

tiers:
  - name: metadata
    hosts: [sasmeta01.example.com]

  - name: compute
    hosts: [sascomp01.example.com, sascomp02.example.com]
    parallel: true

  - name: midtier
    hosts: [sasmid01.example.com]
    configdir: /opt/sas/configmid/Lev1
    retries: 150`),
  P('Each tier begins with a dash (**- name:**). The indentation is significant: settings that belong to a tier are indented under it. Use spaces, not tab characters.'),
  P('The name of the configuration file matters: SAS_lsm uses it (without the .yaml extension) to name the directory where reports for that deployment are written. For example, reports for **prod.yaml** are written to *status_root*/prod/.'),

  H3('Settings'),
  P('The **settings** section controls SAS_lsm itself. Every setting is optional.'),
  Tbl([
    ['Setting', 'Definition', 'Example Value'],
    ['status_root', 'Root directory for reports, collected logs, and the lock file on the SAS_lsm controller. *Default: /var/tmp/sas_lsm.* Do not use /tmp, which might be cleared when the machine restarts.', '/usr/local/etc/SAS/reports'],
    ['admin_email', 'E-mail address or addresses of your deployment administrators. SAS_lsm sends the report after every start and stop action, and after a status action that finds a problem. Requires the mailx command on the controller.', '[sasadmin@example.com, oncall@example.com]'],
    ['mail_command', 'Command used to send e-mail. *Default: mailx.*', 'mailx'],
    ['archive_reports', 'Keep a date-stamped copy of every start and stop report under *status_root*/*config-name*/archive/. *Default: true.*', 'true'],
    ['collect_logs', 'When a tier fails, copy its recent log files to the controller, analyze them, and create a support bundle. Same as adding --collect-logs to every command. *Default: false.*', 'true'],
    ['log_minutes', 'Collect only log files that changed within this many minutes. *Default: 60.*', '60'],
    ['log_max_lines', 'Number of lines to copy from the end of each log file. *Default: 5000.*', '5000'],
    ['ssh_options', 'Options added to every SSH command. *The default is suitable for most sites.*', '[-o, BatchMode=yes, -o, ConnectTimeout=10, -o, StrictHostKeyChecking=accept-new]'],
    ['ssh_multiplex', 'Reuse one SSH connection per host for each run, which makes runs much faster. Set to false if the home directory is on a file system that does not support this (some NFS file systems). *Default: true.*', 'true'],
  ], [1800, 4660, 2900], { mono: [0, 2] }),
  Gap(),

  H3('Defaults and Tier Settings'),
  P('These settings can be placed in the **defaults** section, where they apply to every tier, or in an individual tier, where they apply to that tier only.'),
  Important('Unlike SAS_lsm 4.x, a value that you set in one tier is **never** passed on to the next tier. Each tier uses its own value if it has one, and otherwise the value from the defaults section.'),
  Tbl([
    ['Setting', 'Definition', 'Example Value'],
    ['user', 'Account that runs the commands for the tier (the SAS installer account). *Default: the account that runs SAS_lsm.*', 'sasinst'],
    ['configdir', 'SAS configuration directory, including the Lev*N* directory. Required for sas-servers and metadata-cluster tiers.', '/opt/sas/config/Lev1'],
    ['parallel', 'Control all hosts of the tier at the same time instead of one at a time. *Default: false.*', 'true'],
    ['retries', 'How many times SAS_lsm checks the status of a tier while it waits for the tier to start or stop. *Default: 10.*', '30'],
    ['retry_interval', 'Seconds between status checks. *Default: 6.*', '10'],
    ['tier_delay', 'Seconds to wait after a tier has started before starting the next tier. *Default: 15.*', '15'],
    ['command_timeout', 'Seconds before SAS_lsm stops waiting for a single command to finish. *Default: 900 (15 minutes).*', '1800'],
    ['ok_exit_codes', 'Exit codes of start, stop, and status commands that SAS_lsm treats as successful. The tier\'s state is judged from the status output, so this rarely needs to change. *Default: [0, 1, 2, 4].*', '[0, 1, 2, 4]'],
  ], [1800, 4860, 2700], { mono: [0, 2] }),
  Gap(),
  P('SAS_lsm waits up to approximately *retries* × *retry_interval* seconds for a tier to start or stop. The defaults allow one minute, which is enough for most metadata and compute tiers. Middle-tier web application servers can take 10 to 20 minutes to start, so increase **retries** for the middle tier (for example, **retries: 150** with the default interval allows 15 minutes).'),

  H3('Tier Definitions'),
  P('These settings define a tier. Only **name** and **hosts** are needed for most tiers.'),
  Tbl([
    ['Setting', 'Description', 'Notes'],
    ['name', 'A short name for the tier, such as metadata or midtier. Used in messages and in commands such as sas_lsm start --from midtier.', 'Required. Must be unique.'],
    ['type', 'sas-servers, metadata-cluster, lsf-grid, or custom.', 'Optional. Default: sas-servers.'],
    ['hosts', 'Host names of the machines in the tier, in square brackets and separated by commas.', 'Required, except for lsf-grid tiers.'],
    ['start\nstop\nstatus', 'Commands that start, stop, and check the tier.', 'Required for custom tiers. Optional for other types (overrides the built-in command).'],
    ['restart', 'Command that restarts the tier.', 'Optional. If not set, SAS_lsm stops and then starts the tier.'],
    ['up_pattern\ndown_pattern', 'Text in the status output that shows a service is up or down.', 'Required for custom tiers.'],
    ['logdir', 'Directory that contains the tier\'s log files, for log collection.', 'Optional. Default: the tier\'s configdir.'],
    ['userexits', 'Scripts to run before or after the tier is started, stopped, or checked.', 'Optional. See "UserExit Framework."'],
    ['userexit_dir', 'A SAS_lsm 4.x-style UserExit script directory on the tier\'s first host.', 'Optional.'],
    ['lsf', 'Platform LSF settings.', 'Required for lsf-grid tiers.'],
  ], [1900, 4460, 3000], { mono: [0] }),
  Gap(),
  P('In commands, you can use **{configdir}** and **{user}**, which are replaced by the tier\'s values, and **{host}**, which is replaced by the name of the host that the command runs on.'),
  P('For host names, the fully qualified domain name (FQDN) is recommended. A short name or alias is acceptable if the SAS_lsm controller can resolve it.'),
];
