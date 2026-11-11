const { H1, H2, H3, P, Note, Important, Bullet, Numbered, NumberedCont, Code, Tbl, Gap } = require('./lib');

module.exports = () => [
  // ------------------------------------------------------------ DEPLOYING
  H1('Deploying SAS_lsm'),
  P('Perform every step in this section as the SAS installer account, on the machine that you have chosen as the SAS_lsm controller. The steps are:'),
  ...Numbered([
    'Download and install SAS_lsm.',
    'Set up passwordless SSH to your SAS machines.',
    'Create a configuration file.',
    'Review and validate the configuration file.',
    'Check every host with the doctor command.',
    'Test a dry run, and then run SAS_lsm.',
  ]),

  H2('Download and Install SAS_lsm'),
  P('SAS_lsm 5 is distributed as one executable file for each operating system. Choose the file that matches the SAS_lsm controller (not the other machines in your deployment):'),
  Tbl([
    ['SAS_lsm Controller Operating System', 'File Name'],
    ['Linux on x86-64 (Red Hat Enterprise Linux, SUSE, Oracle Linux, and others), including FIPS-enabled systems', 'sas_lsm-linux-amd64'],
    ['Linux on ARM64', 'sas_lsm-linux-arm64'],
    ['Linux on IBM POWER (little endian)', 'sas_lsm-linux-ppc64le'],
    ['IBM AIX 7.2 or later', 'sas_lsm-aix-ppc64'],
    ['Oracle Solaris 11 on x86-64', 'sas_lsm-solaris-amd64'],
  ], [5860, 3500], { mono: [1] }),
  Gap(),
  Note('Solaris on SPARC and HP-UX are not supported as SAS_lsm controllers. If your SAS deployment runs on these platforms, you can run the SAS_lsm controller on a Linux machine that can reach your SAS machines by SSH, or continue to use SAS_lsm 4.x.'),
  P('To install SAS_lsm:'),
  ...Numbered([
    'Download the file for your platform from the SAS_lsm GitHub project page. Under the newest release, locate the **Assets** list.',
    'Transfer the file to the SAS_lsm controller.',
    'Create a directory for SAS_lsm, copy the file into it with the name sas_lsm, and make it executable:',
  ]),
  Code(
`mkdir -p /opt/sas/utilities/sas_lsm
cp sas_lsm-linux-amd64 /opt/sas/utilities/sas_lsm/sas_lsm
chmod 755 /opt/sas/utilities/sas_lsm/sas_lsm`),
  ...NumberedCont(['Verify the installation:']),
  Code(`/opt/sas/utilities/sas_lsm/sas_lsm version`),
  P('Do not place SAS_lsm in the Utilities directory under your SAS configuration directory, because SAS deployment tools manage that directory.'),
  P('Optionally, add the SAS_lsm directory to the SAS installer account\'s PATH, and set a default configuration file so that you do not need to type -c on every command:'),
  Code(
`echo 'export PATH=/opt/sas/utilities/sas_lsm:$PATH' >> ~/.bash_profile
echo 'export SAS_LSM_CONFIG=/opt/sas/utilities/sas_lsm/prod.yaml' >> ~/.bash_profile`),
  P('The examples in the rest of this guide assume that sas_lsm is in your PATH.'),

  // ------------------------------------------------------------ SSH
  H2('Set Up Passwordless SSH'),
  P('If every tier in your deployment runs on the SAS_lsm controller under the account that runs SAS_lsm (a single-machine deployment), you can skip this step.'),
  P('Otherwise, run the ssh-setup command with the host names of every machine in your SAS deployment. You do not need a configuration file yet:'),
  Code(`sas_lsm ssh-setup --user sasinst \\
    --hosts sasmeta01.example.com,sascomp01.example.com,sasmid01.example.com`),
  P('If a machine uses a different account, write it as *account*@*host*, for example **sasmid@sasmid01.example.com**. After you have a configuration file, you can also run **sas_lsm ssh-setup -c prod.yaml** to set up every account and host in that file.'),
  P('The ssh-setup command performs these steps:'),
  ...Numbered([
    '**Chooses an SSH key.** It uses an existing key of the account, or creates a new key without a passphrase. On FIPS-enabled systems, it never uses Ed25519 keys (see "SSH Keys and FIPS Mode").',
    '**Checks each host** to see whether it already accepts the key, and lists each host with its state (see the table below).',
    '**Explains what happens next** before it asks for anything: how many hosts need the key, and that you will be asked for each account\'s password once.',
    '**Installs the key** on each host that needs it. You are prompted for that account\'s password; the prompt names the host and the account.',
    '**Verifies** that each host now accepts the key, and prints a summary.',
  ]),
  P('Each host is listed with one of these states:'),
  Tbl([
    ['State', 'Meaning', 'What Happens Next'],
    ['[ready]', 'The key is already installed.', 'Nothing. The host is skipped.'],
    ['[needs key]', 'The host can be reached, but the key is not installed yet. This is expected the first time that you run ssh-setup.', 'You are asked for the account\'s password once, and the key is installed.'],
    ['[problem]', 'The host name cannot be resolved, or the host or its SSH server cannot be reached.', 'The host is skipped, and a suggested fix is shown. A password would not help.'],
    ['[host key]', 'The host\'s SSH key has changed since it was first recorded.', 'The host is skipped. Confirm with your system administrator that the change is expected, run ssh-keygen -R *host*, and then run ssh-setup again.'],
  ], [1600, 4060, 3700], { mono: [0] }),
  Gap(),
  P('Example output from a first run:'),
  Code(
`Using existing SSH key /home/sasinst/.ssh/id_ecdsa.pub

Checking which hosts already accept this key...

  [ready]      sasinst@sasmeta01.example.com   key already installed
  [needs key]  sasinst@sascomp01.example.com   key not installed yet (expected)
  [needs key]  sasinst@sasmid01.example.com    key not installed yet (expected)

2 hosts need the key. For each one, ssh-copy-id will ask for that account's
password once, then install the key so SAS_lsm can log in without a password.
Continue? [y/N] y

==> [1/2] sasinst@sascomp01.example.com: enter the password for sasinst when prompted
sasinst@sascomp01.example.com's password:
Number of key(s) added: 1
    verified: sasinst@sascomp01.example.com now accepts the key

==> [2/2] sasinst@sasmid01.example.com: enter the password for sasinst when prompted
sasinst@sasmid01.example.com's password:
Number of key(s) added: 1
    verified: sasinst@sasmid01.example.com now accepts the key

Summary
  ready:    3 host(s) (1 already set up, 2 installed now)

All hosts are ready.`),
  P('It is safe to run ssh-setup again. Hosts that are already set up are skipped, so after you fix a host marked [problem] or [host key], simply run the same command again. The command ends with exit code 0 when every host is ready, and 1 if any host was skipped or failed.'),
  P('The ssh-setup command has these options:'),
  Tbl([
    ['Option', 'Description'],
    ['--hosts H1,USER@H2,...', 'The hosts to set up. Use this option or -c, not both.'],
    ['--user USER', 'The account for hosts that are listed without USER@. Default: the account that runs SAS_lsm.'],
    ['-c FILE', 'Set up every account and host in a configuration file.'],
    ['--key-type TYPE', 'auto (default), ecdsa, rsa, or ed25519. See "SSH Keys and FIPS Mode."'],
    ['--yes', 'Do not ask for confirmation before installing keys. You are still prompted for passwords.'],
    ['--verbose', 'Also show the SSH program\'s own message for each host that is not ready. Useful when a host is marked [problem].'],
  ], [3000, 6360], { mono: [0] }),
  Gap(),

  H3('SSH Keys and FIPS Mode'),
  P('The ssh-setup command checks whether the SAS_lsm controller runs in FIPS mode, by reading /proc/sys/crypto/fips_enabled and, on RHEL 8 and later, the system crypto policy. In FIPS mode, Ed25519 keys are not allowed, so the key is chosen as follows:'),
  Tbl([
    ['--key-type', 'Not in FIPS Mode', 'In FIPS Mode'],
    ['auto (default)', 'Uses or creates an Ed25519 key, then ECDSA, then RSA', 'Uses or creates an ECDSA (P-384) key, then RSA (4096 bits). Ed25519 is never used.'],
    ['ecdsa or rsa', 'That key type', 'That key type'],
    ['ed25519', 'Ed25519', 'Refused with an error message'],
  ], [2200, 3400, 3760], { mono: [0] }),
  Gap(),
  P('An existing private key is never overwritten. If the file ~/.ssh/id_ed25519 already exists on a FIPS-enabled system, it is left in place, and the ssh command might print the harmless warning "Ed25519 keys are not allowed in FIPS mode." To stop the warning, move that key file to another directory. SAS_lsm uses the ECDSA or RSA key automatically.'),

  H3('Setting Up SSH Manually'),
  P('If the ssh-copy-id command is not available on the SAS_lsm controller (for example, on some AIX and Solaris systems), ssh-setup tells you so before it asks for any passwords. Install the key manually with these commands. On FIPS-enabled systems, use **-t ecdsa -b 384** instead of **-t ed25519**.'),
  Code(
`# On the SAS_lsm controller, as the SAS installer account (once):
ssh-keygen -t ed25519 -N "" -f ~/.ssh/id_ed25519
cat ~/.ssh/id_ed25519.pub              # copy this line

# On each tier host, as the tier's account:
mkdir -p ~/.ssh && chmod 700 ~/.ssh
echo "paste-the-public-key-line-here" >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys

# Back on the controller - this must print the host name without a password prompt:
ssh -o BatchMode=yes sasinst@sascomp01.example.com hostname`),

  // ------------------------------------------------------------ CONFIG
  H2('Create the Configuration File'),
  P('There are three ways to create your configuration file. Choose the one that fits your situation:'),
  Tbl([
    ['Method', 'Use When'],
    ['A. Convert your SAS_lsm 4.x configuration file', 'You already use SAS_lsm 4.x.'],
    ['B. Discover your deployment (recommended for new users)', 'You are new to SAS_lsm, and your SAS deployment uses standard SAS configuration directories.'],
    ['C. Start from an example file', 'You prefer to write the file yourself, or your deployment is unusual.'],
  ], [4200, 5160]),
  Gap(),

  H3('Method A: Convert a SAS_lsm 4.x Configuration File'),
  P('The convert command reads your SAS_lsm 4.x configuration file and writes an equivalent SAS_lsm 5 file. It also lists anything that you should review:'),
  Code(`sas_lsm convert /opt/sas/utilities/SAS_lsm.cfg > /opt/sas/utilities/sas_lsm/prod.yaml`),
  P('The convert command requires the BASH shell, because a SAS_lsm 4.x configuration file is a BASH script. It reads the file in the same way that SAS_lsm 4.x did. See "Migrating from SAS_lsm 4.x" for details of what is converted.'),

  H3('Method B: Discover Your Deployment'),
  P('The discover command connects to each machine that you list, looks in the SAS configuration directory to see which SAS components are there, and writes a draft configuration file with the tiers in the usual order: metadata, compute, and then middle tier. It does not change anything on the machines. Passwordless SSH must already work, so run ssh-setup first (see "Set Up Passwordless SSH").'),
  P('You need the following information before you run the command:'),
  Tbl([
    ['Value Required', 'Example Answer', 'Notes'],
    ['The host name of each machine in the SAS deployment', 'sasmeta01.example.com,\nsascomp01.example.com,\nsasmid01.example.com', '*Separate the names with commas. Use the same list that you gave to ssh-setup.'],
    ['The SAS configuration directory', '/opt/sas/config/Lev1', '*Include Lev N. If the middle tier uses a different directory, fix that tier in the draft afterward.'],
    ['The SAS installer account', 'sasinst', '*See "Additional Considerations."'],
    ['The path and name of the configuration file', '/opt/sas/utilities/sas_lsm/prod.yaml', '*The file name is used to name the report directory.'],
  ], [2900, 3360, 3100], { mono: [1, 2] }),
  Gap(),
  Code(
`sas_lsm discover --hosts sasmeta01.example.com,sascomp01.example.com,sasmid01.example.com \\
                 --configdir /opt/sas/config/Lev1 --user sasinst \\
                 > /opt/sas/utilities/sas_lsm/prod.yaml`),
  P('Machines that could not be reached, or that have no SAS services in the configuration directory, are listed as comments at the end of the file. Review the draft carefully: discovery bases its decisions on which directories exist, and it cannot know about services that are not under the SAS configuration directory (for example, SAS LASR Analytic Server or third-party services).'),

  H3('Method C: Start from an Example File'),
  P('Copy the example in "The SAS_lsm Configuration File" (earlier in this guide), or one of the examples in the "Example Configuration Files" appendix, and change the host names, account, and directory to match your deployment.'),

  H2('Review and Modify Your Configuration File'),
  P('Open the configuration file in a text editor such as vi or nano, and check it for accuracy. Pay particular attention to these items:'),
  Bullet('The tiers are in the order in which they must start.'),
  Bullet('The host names are correct and can be resolved from the SAS_lsm controller.'),
  Bullet('The **user** and **configdir** values are correct for every tier.'),
  P('If you add hosts or accounts that you did not give to ssh-setup, run **sas_lsm ssh-setup -c prod.yaml**. Hosts that are already set up are skipped.', { spacing: { before: 120, after: 140 } }),
  P('For most standard deployments, no further changes are needed. The following sections describe common changes.'),

  H3('Use a Different Configuration Directory or Account for One Tier'),
  P('If one tier uses a different SAS configuration directory or account (for example, a middle tier installed in /opt/sas/configmid/Lev1), set the value in that tier only. Other tiers continue to use the value in the defaults section:'),
  Code(
`defaults:
  user: sasinst
  configdir: /opt/sas/config/Lev1

tiers:
  - name: metadata
    hosts: [sasmeta01.example.com]
  - name: compute
    hosts: [sascomp01.example.com]
  - name: midtier
    hosts: [sasmid01.example.com]
    configdir: /opt/sas/configmid/Lev1     # this tier only`),

  H3('Two Tiers on One Machine'),
  P('If one machine hosts two tiers that must be controlled separately, such as a shared compute and middle-tier server with two configuration directories, list the same host name in both tiers and set a different **configdir** in each.'),

  H3('Control Several Machines at Once'),
  P('A tier can contain several machines. By default, SAS_lsm controls them one at a time. Add **parallel: true** to control them at the same time, which is faster. Do not use parallel for machines that must start in a particular order, such as a clustered middle tier, unless you know that this is safe for your deployment.'),

  H3('Enable Control of a Clustered SAS Metadata Server'),
  P('Use one tier of type metadata-cluster that lists every node, with the first-deployed node first:'),
  Code(
`  - name: metadata
    type: metadata-cluster
    hosts: [sasmeta01.example.com, sasmeta02.example.com, sasmeta03.example.com]`),
  P('A configuration file can contain only one metadata-cluster tier, and it must list at least three hosts.'),

  H3('Enable Control of a Platform LSF Grid'),
  P('Add a tier of type lsf-grid, and supply the LSF details in an **lsf** section. Do not list **hosts** for this tier type.'),
  Code(
`  - name: lsf
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
      ego_controls_daemons: false
      sudo: true`),
  Tbl([
    ['LSF Setting', 'Description'],
    ['master', 'Host name of the LSF master. Required.'],
    ['nodes', 'Host names of the other LSF hosts.'],
    ['envdir', 'The LSF_ENVDIR directory, which contains profile.lsf. Required.'],
    ['serverdir', 'The LSF_SERVERDIR directory, which contains the LSF daemon programs. Required.'],
    ['pm_host, pm_envdir, pm_serverdir', 'Optional. The Process Manager host, its JS_ENVDIR directory (which contains profile.js), and its JS_SERVERDIR directory.'],
    ['ego_controls_daemons', 'Set to true if EGO starts the res and sbatchd daemons. SAS_lsm then starts only the LIM daemon on each host. Default: false.'],
    ['sudo', 'Run LSF commands through sudo. The LSF daemons run as root, so this is usually required. Default: true.'],
  ], [3000, 6360], { mono: [0] }),
  Gap(),
  P('To start the grid, SAS_lsm starts LSF on the master, then on each node, and then starts Process Manager. To stop the grid, it reverses the order. SAS_lsm checks the status of the grid by looking for the LSF daemon processes on each host.'),
  P('Because the LSF daemons run as root, the SAS installer account must be able to run the LSF commands with sudo without a password. Ask your system administrator to add a rule like the following one to the sudoers file on every LSF host (adjust the paths to match your LSF directories):'),
  Code(
`sasinst ALL=(root) NOPASSWD: /bin/sh -c . /opt/sas/lsf/conf/profile.lsf *, \\
                            /bin/sh -c . /opt/sas/pm/conf/profile.js *`),
  P('On some older Linux releases, you must also add **Defaults:sasinst !requiretty**. The doctor command checks that passwordless sudo works.'),
  P('The lsf-grid tier controls only the LSF daemons. Put the SAS services on the grid machines (for example, the object spawner) in a separate sas-servers tier after the lsf-grid tier. Process Manager under EGO control (egosh service start ProcessManager) is not built in. If you need it, use a UserExit script or a custom tier.'),

  H3('Enable Control of Third-Party Services'),
  P('Use a tier of type custom. You provide the commands and the text that the status command prints when the service is up and when it is down. The text is a regular expression, and it is case-sensitive:'),
  Code(
`  - name: reporting-app
    type: custom
    hosts: [app01.example.com]
    user: appadm
    start:  "/opt/app/bin/appctl start"
    stop:   "/opt/app/bin/appctl stop"
    status: "/opt/app/bin/appctl status"
    up_pattern: "is running"
    down_pattern: "is not running|is stopped"`),
  P('Write commands exactly as you would type them at a shell prompt on the tier host, and enclose them in double quotation marks. If a start command leaves a program running in the background, redirect its output, for example: **"nohup /opt/app/bin/server >/dev/null 2>&1 &"**. Otherwise, SAS_lsm waits up to 5 seconds for the command\'s output to end.'),

  H3('Enable Control of SAS® LASR™ Analytic Server'),
  P('There are two methods for controlling LASR servers with SAS_lsm. The recommended method uses the LASROps utility, which controls LASR servers through the LASR API. The legacy SAS_lsm method is still available for MPP (multi-machine) LASR deployments, but it is more involved to configure.'),
  P('**Control LASR with the LASROps utility (recommended).** Download and configure the LASROps utility from its GitHub project page (https://github.com/ajforeman/LASROps), following the instructions in its README. Test the utility on its own to verify that it can start, stop, and check the status of your LASR servers. Then add it to your configuration file as a custom tier:'),
  Code(
`  - name: lasr
    type: custom
    hosts: [lasrhead01.example.com]
    start:  "/opt/sas/config/Utilities/LASROps/LASROps.sh start"
    stop:   "/opt/sas/config/Utilities/LASROps/LASROps.sh stop"
    status: "/opt/sas/config/Utilities/LASROps/LASROps.sh status"
    up_pattern: "is UP"
    down_pattern: "is DOWN"
    logdir: /opt/sas/config/Utilities/LASROps`),
  P('**Control LASR with the legacy SAS_lsm method (MPP LASR only).** Generate the LASR control programs (mpp_start.sas, mpp_stop.sas, mpp_checklasr.sas, and the instance-specific configuration files) by following the "Enable Control of SAS LASR Analytic Server" section of the SAS_lsm 4.x guide, using the SAS_lsm_LASR.tar file from the SAS_lsm 4.x package. Then add one custom tier for each LASR port:'),
  Code(
`  - name: lasr-10011
    type: custom
    hosts: [sasapp-2.example.com]
    user: sasdemo
    start:  "/opt/sas94/SASHome/SASFoundation/9.4/sas /usr/local/etc/SAS/LASR/mpp_start.sas -log /usr/local/etc/SAS/LASR/logs/10011 -sysparm 10011"
    stop:   "/opt/sas94/SASHome/SASFoundation/9.4/sas /usr/local/etc/SAS/LASR/mpp_stop.sas -log /usr/local/etc/SAS/LASR/logs/10011 -sysparm 10011"
    status: "/opt/sas94/SASHome/SASFoundation/9.4/sas /usr/local/etc/SAS/LASR/mpp_checklasr.sas -log /usr/local/etc/SAS/LASR/logs/10011 -sysparm 10011"
    up_pattern: "LASR UP"
    down_pattern: "LASR DOWN"
    logdir: /usr/local/etc/SAS/LASR/logs/10011`),

  H2('Validate the Configuration File'),
  P('Run the validate command after every change to the configuration file:'),
  Code(`sas_lsm validate -c /opt/sas/utilities/sas_lsm/prod.yaml`),
  P('If the file is valid, SAS_lsm lists the tiers in order. If it is not valid, SAS_lsm lists every problem that it found, with line numbers, so that you can correct them all at once. For example:'),
  Code(
`configuration prod.yaml is not valid:
  - line 14: tier 2 (compute): unknown setting "paralel" (allowed: configdir, ...)
  - line 18: tier 3 (midtier): 'hosts' must list at least one host`),
  P('Validation checks the file itself (spelling of settings, required values, and so on). It does not check the machines. That is the job of the doctor command, described below.'),
  P('Follow these rules when you edit the file:'),
  Bullet('Indent with spaces, never with tab characters.'),
  Bullet('Write lists in square brackets, such as **[host1, host2]**, or as indented lines that begin with a dash.'),
  Bullet('Enclose commands, and any value that contains a colon followed by a space or the # character, in double quotation marks. Inside double quotation marks, write \\" for a quotation mark and \\\\ for a backslash.'),
  Bullet('A # character starts a comment, unless it is inside quotation marks.'),

  H2('Check Every Host'),
  P('The doctor command connects to every host in the configuration file and checks that SAS_lsm can work there:'),
  Code(`sas_lsm doctor -c /opt/sas/utilities/sas_lsm/prod.yaml`),
  P('It checks the following items:'),
  Bullet('SAS_lsm is not running as root, and the report directory can be written.'),
  Bullet('Each host can be reached without a password, and you are logged on as the expected account.'),
  Bullet('The SAS scripts (sas.servers or MetadataServer.sh) exist and can be run in the configuration directory.'),
  Bullet('For lsf-grid tiers, profile.lsf can be read and passwordless sudo works.'),
  Bullet('UserExit scripts and directories exist and can be run.'),
  P('Every failed check includes a suggested fix. Correct any failures, and run doctor again until all checks pass:', { spacing: { before: 120, after: 140 } }),
  Code(
`sasinst@sascomp01.example.com
  [ok  ] connect without a password
  [ok  ] logged in as the expected user - sasinst
  [FAIL] sas.servers is executable (2:compute)
         fix: check this tier's hosts and configdir (/opt/sas/config/Lev1)`),

  H2('Test a Dry Run'),
  P('Before the first real run, use --dry-run to see exactly what SAS_lsm would do, in what order, without running anything:'),
  Code(
`sas_lsm start -c /opt/sas/utilities/sas_lsm/prod.yaml --dry-run
sas_lsm stop  -c /opt/sas/utilities/sas_lsm/prod.yaml --dry-run`),
  P('Then check the current status of your deployment. This is a safe way to confirm that SAS_lsm reads the status of every tier correctly:'),
  Code(`sas_lsm status -c /opt/sas/utilities/sas_lsm/prod.yaml`),
  P('If a tier shows as UNKNOWN even though its machines can be reached, the status output did not contain the expected text. See "Troubleshooting."'),
];
