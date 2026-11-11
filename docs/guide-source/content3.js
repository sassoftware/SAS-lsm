const { H1, H2, P, Note, Important, Bullet, Numbered, Code, Tbl, Gap } = require('./lib');

module.exports = () => [
  // ------------------------------------------------------------ USEREXITS
  H1('UserExit Framework'),
  H2('Capabilities'),
  P('With the UserExit framework, you can run your own scripts at specific points while SAS_lsm starts, stops, or checks a tier. This gives administrators an easy way to perform additional tasks when SAS services are started and stopped, and a simple way to connect to third-party tools.'),
  P('Using the UserExit framework is optional. You can run UserExit scripts at these points for each tier:'),
  Tbl([
    ['Event', 'When the Script Runs'],
    ['pre_start', 'Before the tier is started'],
    ['post_start_success', 'After the tier has started successfully'],
    ['post_start_failure', 'After the tier has failed to start'],
    ['pre_stop', 'Before the tier is stopped'],
    ['post_stop_success', 'After the tier has stopped successfully'],
    ['post_stop_failure', 'After the tier has failed to stop'],
    ['pre_status', 'Before the tier\'s status is checked'],
    ['post_status', 'After the tier\'s status is checked'],
  ], [3200, 6160], { mono: [0] }),
  Gap(),
  P('A UserExit script can be marked as **required**. If a required script fails, SAS_lsm does not continue: a failed pre_ script stops the action before the tier is touched, and a failed post_ script marks the tier as failed, so later tiers are not started or stopped. If a script is not required, SAS_lsm reports a warning when it fails and continues.'),
  P('Here are some examples of tasks that you can implement with UserExit scripts:'),
  Bullet('Before a tier is started, verify that required network ports are not in use.'),
  Bullet('After a tier is started successfully, start the SAS Deployment Agent service on that tier.'),
  Bullet('After a tier fails to start, create a list of configuration files that were changed in the past day.'),
  Bullet('Before a tier is stopped, e-mail users to tell them that SAS services are being restarted.'),
  Bullet('After a tier is stopped successfully, back up the SASHOME and SAS configuration directories.'),
  Bullet('After a tier fails to stop, list any orphaned processes on the host.'),

  H2('Configuration'),
  P('List UserExit scripts in the tier\'s **userexits** section, under the event at which they should run. Scripts run in the order listed:'),
  Code(
`  - name: compute
    hosts: [sascomp01.example.com, sascomp02.example.com]
    userexits:
      pre_stop:
        - /opt/sas/utilities/userexits/notify_users.sh
        - run: /opt/sas/utilities/userexits/drain_jobs.sh {host}
          required: true
      post_start_success:
        - run: /opt/sas/utilities/userexits/start_deployment_agent.sh
          on: all`),
  P('A script can be listed by its path alone, or with the following options:'),
  Tbl([
    ['Option', 'Description', 'Default'],
    ['run', 'The command to run, with any arguments. {host} is replaced with the host name.', '(required)'],
    ['required', 'true if a failure of this script must stop the SAS_lsm action.', 'false'],
    ['on', 'first to run the script on the tier\'s first host only, or all to run it on every host of the tier.', 'first'],
  ], [1800, 5560, 2000], { mono: [0, 2] }),
  Gap(),
  P('UserExit scripts run on the tier\'s host, under the tier\'s account. The script must exist on that host and be executable by that account. The doctor command checks this.'),

  H2('Using a SAS_lsm 4.x UserExit Directory'),
  P('If you already have UserExit scripts that follow the SAS_lsm 4.x naming convention, you can keep them where they are. Set **userexit_dir** in the tier to the directory on the tier\'s first host that contains the scripts:'),
  Code(
`  - name: compute
    hosts: [sascomp01.example.com]
    userexit_dir: /opt/sas/userexits/compute`),
  P('Script file names follow this syntax:'),
  Code(`XX_[PRE|POST]_[START|STOP|STATUS]_[SUCCESS|FAILURE]_[REQUIRED]_identifying-name`),
  Bullet('*XX* is a number that sets the order in which scripts run (01, 02, 03, and so on).'),
  Bullet('PRE or POST, and START, STOP, or STATUS, are required.'),
  Bullet('SUCCESS or FAILURE is optional, and applies to POST START and POST STOP scripts only. A POST START or POST STOP script **without** SUCCESS or FAILURE runs after either result.'),
  Bullet('REQUIRED is optional, and marks the script as required.'),
  Bullet('The *identifying-name* is any text that helps you recognize the script.'),
  P('File names can be in uppercase, lowercase, or mixed case. Scripts in the directory run after any scripts listed in the tier\'s userexits section.', { spacing: { before: 120, after: 140 } }),
  P('To see which scripts will run for each event, use the userexits command. It replaces the userExit_tester.bash script from SAS_lsm 4.x, and it checks the directory on the tier\'s host:'),
  Code(`sas_lsm userexits -c /opt/sas/utilities/sas_lsm/prod.yaml --tier compute`),
  Code(
`tier 2:compute
  pre_stop             /opt/sas/userexits/compute/01_PRE_STOP_notify_users.sh   <- /opt/sas/userexits/compute
  post_start_success   /opt/sas/utilities/userexits/start_deployment_agent.sh [all hosts]   <- config
                       /opt/sas/userexits/compute/02_POST_START_agent.sh   <- /opt/sas/userexits/compute`),

  H2('Custom Scripts'),
  P('SAS_lsm determines whether a UserExit script succeeded from the script\'s **exit code**. An exit code of **0** means success. Any other exit code means failure.'),
  Important('This is a change from SAS_lsm 4.x, which also treated exit codes 1, 2, and 4 as success. If you have UserExit scripts that return these codes on purpose, change them to return 0, or use a wrapper script like the examples below.'),
  P('If you want to run a third-party command that does not return exit code 0 on success, use a translation wrapper script:'),
  P('**Example 1: Decide success from the command\'s output**'),
  Code(
`#!/bin/sh
OUTPUT=$(/path/to/some/service/serviceagent.sh start 2>&1)
case "$OUTPUT" in
  *"started successfully"*) exit 0 ;;
  *) echo "$OUTPUT"; exit 5 ;;
esac`),
  P('**Example 2: Translate exit codes**'),
  Code(
`#!/bin/sh
/path/to/some/service/serviceagent.sh start
RC=$?
if [ $RC -eq 0 ] || [ $RC -eq 4 ]; then exit 0; fi   # 4 is acceptable for this tool
exit 5`),
  P('Anything that a UserExit script prints is written to the SAS_lsm report, so you can include messages that will help you understand what happened.'),

  // ------------------------------------------------------------ RUNNING
  H1('Running SAS_lsm Commands'),
  P('Every SAS_lsm action runs as one command. Only one action can run per command, and only one SAS_lsm action can run at a time for each configuration file.'),
  Tbl([
    ['Command', 'Action Performed', 'Statement of Action'],
    ['start --from TIER', 'Start', 'Starts services, beginning at tier TIER and continuing to the last tier. If --from is omitted, starts all tiers.'],
    ['stop --to TIER', 'Stop', 'Stops services, from the last tier down to and including tier TIER. If --to is omitted, stops all tiers.'],
    ['status', 'Status', 'Checks the current status of services on all tiers.'],
  ], [2400, 1700, 5260], { mono: [0] }),
  Gap(),
  P('*TIER* can be a tier number (1 is the first tier in the file) or a tier name, such as **--from midtier**.'),
  P('SAS_lsm checks that the tiers that are not part of the action are in the proper state to support it. For example, suppose that you want to start a deployment from tier 3:'),
  Code(`sas_lsm start --from 3 -c /opt/sas/utilities/sas_lsm/prod.yaml`),
  P('To start the deployment at tier 3 safely, tiers 1 and 2 must already be running, and tiers 4 and later must be stopped. SAS_lsm checks the status of every other tier first. If any tier is not in the expected state, SAS_lsm reports a failure and starts nothing. If tier 3 itself is already running, SAS_lsm restarts it.'),
  P('While it starts each tier, SAS_lsm waits until every host reports that its services are running before it starts the next tier. If a tier does not start, SAS_lsm does not start the tiers after it. Similarly, if a tier does not stop, SAS_lsm leaves the tiers before it running, so that, for example, the metadata server remains available while you investigate.'),
  P('The table below shows some example commands and describes what they do:'),
  Tbl([
    ['Full Command', 'Statement of Action'],
    ['sas_lsm start -c prod.yaml', 'Starts services on all tiers.'],
    ['sas_lsm start --from midtier -c prod.yaml', 'Starts services beginning at the tier named midtier.'],
    ['sas_lsm stop -c prod.yaml', 'Stops services on all tiers.'],
    ['sas_lsm stop --to 2 -c prod.yaml', 'Stops services down to and including tier 2.'],
    ['sas_lsm status -c prod.yaml', 'Checks the current status of services on all tiers.'],
    ['sas_lsm start -c prod.yaml --dry-run', 'Shows what a start would do, without doing it.'],
    ['sas_lsm stop -c prod.yaml --collect-logs', 'Stops all tiers, and collects logs if a tier fails to stop.'],
  ], [4700, 4660], { mono: [0] }),
  Gap(),

  H2('Command Options'),
  Tbl([
    ['Option', 'Applies To', 'Description'],
    ['-c FILE', 'All commands', 'The configuration file. If omitted, SAS_lsm uses the SAS_LSM_CONFIG environment variable, and then sas_lsm.yaml in the current directory.'],
    ['--dry-run', 'start, stop', 'Show what would be done, without doing it.'],
    ['--collect-logs', 'start, stop, status', 'If a tier fails, collect its logs and create a support bundle.'],
    ['--no-userexits', 'start, stop, status', 'Do not run any UserExit scripts for this run.'],
    ['-q', 'start, stop, status', 'Do not write to the terminal. The report file is still written. Use this for scheduled runs.'],
  ], [2100, 2100, 5160], { mono: [0] }),
  Gap(),
  P('The SAS_lsm 4.x command options still work, so existing scripts and cron entries continue to work after you switch to SAS_lsm 5 (with a converted configuration file):'),
  Tbl([
    ['SAS_lsm 4.x', 'SAS_lsm 5 Equivalent'],
    ['SAS_lsm -a 5 -c FILE', 'sas_lsm start --from 5 -c FILE'],
    ['SAS_lsm -o 5 -c FILE', 'sas_lsm stop --to 5 -c FILE'],
    ['SAS_lsm -s -c FILE', 'sas_lsm status -c FILE'],
    ['-e', '--collect-logs'],
    ['-x', '--no-userexits'],
  ], [4680, 4680], { mono: [0, 1] }),
  Gap(),

  H2('Exit Codes'),
  P('Every SAS_lsm command returns an exit code that scripts and schedulers can test:'),
  Tbl([
    ['Exit Code', 'Meaning'],
    ['0', 'Success.'],
    ['1', 'An action failed, a status check after a start found tiers that are not fully running, or ssh-setup left a host that is not ready.'],
    ['2', 'A start was refused because the dependency check failed. Nothing was changed.'],
    ['3', 'The configuration file or the command is not valid.'],
    ['4', 'The state of at least one tier could not be determined (for example, a host could not be reached).'],
    ['5', 'Another SAS_lsm action is already running for this configuration file.'],
  ], [1500, 7860], { mono: [0] }),
  Gap(),

  H2('How SAS_lsm Decides Whether a Tier Is Running'),
  P('SAS_lsm runs the tier\'s status command on each host and reads the output line by line:'),
  Tbl([
    ['Output Contains up_pattern', 'Output Contains down_pattern', 'Host State'],
    ['Yes', 'No', 'UP'],
    ['No', 'Yes', 'DOWN'],
    ['Yes', 'Yes', 'PARTIAL (some services are up and some are down)'],
    ['No', 'No', 'UNKNOWN (unexpected output, or the host could not be reached)'],
  ], [2900, 2900, 3560]),
  Gap(),
  P('A tier is UP when all its hosts are UP, and DOWN when all its hosts are DOWN. A metadata-cluster tier follows the quorum rule described in "SAS_lsm Tier Types."'),
  P('The status action reports the state of every tier. Its exit code depends on the last start or stop action. After a start, any tier that is not UP is a failure (exit code 1). After a stop, a stopped or partially stopped deployment is expected and is not a failure (exit code 0). If any tier is UNKNOWN, the exit code is 4.'),

  // ------------------------------------------------------------ REPORTS
  H1('Centralized Report Output and Log Collection'),
  P('SAS_lsm stores reports and collected logs on the SAS_lsm controller, in a directory for each configuration file: *status_root*/*config-name*/. For example, for the configuration file prod.yaml and the default status_root, the directory is /var/tmp/sas_lsm/prod/. The following table shows what you can find there:'),
  Tbl([
    ['Artifact', 'Description'],
    ['report.status', 'Details of the most recent status action'],
    ['report.start', 'Details of the most recent start action'],
    ['report.stop', 'Details of the most recent stop action'],
    ['archive/', 'Date-stamped copies of every start and stop report (when archive_reports is true)'],
    ['logfiles/', 'Copies of the recent log files from the failed tier, organized by host and tier *'],
    ['logfiles.ANALYSIS', 'The most frequent ERROR messages in the collected logs, and every ERROR line by file. Useful when you open a case with SAS Technical Support. *'],
    ['support-bundle-YYYYMMDD-HHMMSS.tar.gz', 'A compressed archive of the reports, collected logs, and analysis, ready to send to SAS Technical Support *'],
  ], [3900, 5460], { mono: [0] }),
  P('* Created only when a tier fails and log collection is enabled (the --collect-logs option or the collect_logs setting).', { spacing: { before: 60, after: 160 } }),
  P('The terminal shows a summary of each action. The report file contains the full details, including every command that SAS_lsm ran and the complete output of each command. When a tier does not behave as expected, the report file is the first place to look.'),
  P('When a start or stop action fails, or a status action finds a problem after a start, and log collection is enabled, SAS_lsm does the following:'),
  ...Numbered([
    'Finds the log files (files with the .log extension) under the failed tier\'s log directory that changed in the last *log_minutes* minutes (60 by default), up to 50 files on each host.',
    'Copies the last *log_max_lines* lines (5,000 by default) of each file to the logfiles/ directory on the controller.',
    'Analyzes the copied logs for ERROR messages, writes logfiles.ANALYSIS, and shows the 10 most frequent errors in the report.',
    'Creates the support bundle.',
  ]),
  P('If you cannot resolve the problem yourself, provide the support bundle to SAS Technical Support. Remove anything confidential first.'),

  // ------------------------------------------------------------ CRON / BOOT
  H1('Scheduling SAS_lsm'),
  H2('SAS_lsm and cron'),
  P('You can schedule SAS_lsm with cron. Run cron jobs as the SAS installer account, and use the -q option so that cron does not e-mail the terminal output:'),
  Code(
`# Check status every 10 minutes. E-mail is sent (via admin_email) only when something is wrong.
*/10 * * * * /opt/sas/utilities/sas_lsm/sas_lsm status -q -c /opt/sas/utilities/sas_lsm/prod.yaml

# Restart SAS every Sunday at 02:00.
0 2 * * 0 /opt/sas/utilities/sas_lsm/sas_lsm stop -q -c /opt/sas/utilities/sas_lsm/prod.yaml && /opt/sas/utilities/sas_lsm/sas_lsm start -q -c /opt/sas/utilities/sas_lsm/prod.yaml`),
  P('SAS_lsm 5 prevents two actions from running at the same time for the same configuration file, whether they were started by cron or by a person. The second action ends immediately with exit code 5. You no longer need a separate copy of the configuration file for cron, as SAS_lsm 4.x recommended.'),
  P('Because a status action after a deliberate stop is not a failure, a regular status check does not send false alarms during planned maintenance, as long as you stop SAS with SAS_lsm.'),

  H2('Starting SAS When the Machine Starts (Linux)'),
  P('On Linux systems that use systemd, you can start and stop SAS with the SAS_lsm controller. Ask your system administrator to create the file /etc/systemd/system/sas-lsm.service on the controller:'),
  Code(
`[Unit]
Description=SAS 9.4 deployment (SAS_lsm)
After=network-online.target sshd.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
User=sasinst
Environment=SAS_LSM_CONFIG=/opt/sas/utilities/sas_lsm/prod.yaml
ExecStart=/opt/sas/utilities/sas_lsm/sas_lsm start -q
ExecStop=/opt/sas/utilities/sas_lsm/sas_lsm stop -q
TimeoutStartSec=0
TimeoutStopSec=1800

[Install]
WantedBy=multi-user.target`),
  P('Then enable it with the command **systemctl daemon-reload && systemctl enable sas-lsm**.'),
  P('In a multi-machine deployment, all the other machines must be running and reachable by SSH before SAS_lsm starts. If they are not, the dependency check finds tiers in an UNKNOWN state, and SAS_lsm starts nothing. Make sure that the controller starts last, or schedule the start for some time after the machines start.'),

  // ------------------------------------------------------------ MIGRATING
  H1('Migrating from SAS_lsm 4.x'),
  P('Follow these steps to move from SAS_lsm 4.x to SAS_lsm 5:'),
  ...Numbered([
    'Install SAS_lsm 5, as described in "Download and Install SAS_lsm." You can keep SAS_lsm 4.x installed while you test.',
    'Convert your configuration file: **sas_lsm convert /path/to/SAS_lsm.cfg > prod.yaml**. Read the review notes that are displayed.',
    'Run **sas_lsm validate**, then **sas_lsm ssh-setup -c prod.yaml** (hosts that already accept your existing key are skipped), then **sas_lsm doctor**, and then **sas_lsm start --dry-run**. Compare the plan with the way that SAS_lsm 4.x controls your deployment.',
    'Run **sas_lsm status** and compare the result with **SAS_lsm -s**.',
    'Change your cron entries and start-up scripts to use the new program and the new configuration file.',
  ]),
  P('Your existing SSH keys continue to work. If the controller is now in FIPS mode and your SAS_lsm 4.x setup used an Ed25519 key, ssh-setup installs an ECDSA key alongside it.'),
  H2('What the convert Command Does'),
  Tbl([
    ['SAS_lsm 4.x', 'SAS_lsm 5'],
    ['MAXTIERS and numbered TIER...[N] variables', 'An ordered list of tiers. Tiers are counted automatically.'],
    ['MDCN tiers and the MDCV tier', 'One metadata-cluster tier'],
    ['A tier whose start command is .../sas.servers start', 'A sas-servers tier with the matching configdir'],
    ['Any other tier', 'A custom tier with the same commands'],
    ['TIERSTOS / TIERSTAS', 'up_pattern / down_pattern'],
    ['TIERNAME', 'A tier name that is easy to type (for example, Mid Tier becomes mid-tier)'],
    ['INSTID, CONFIGDIR', 'defaults: user and configdir, with per-tier values where tiers differed'],
    ['MAXRETRY, SLEEP', 'defaults: retries and retry_interval'],
    ['STATUSROOT, SASADMIN, SSHOPTIONS', 'settings: status_root, admin_email, ssh_options'],
    ['StrictHostKeyChecking=no', 'Replaced by StrictHostKeyChecking=accept-new'],
    ['TIEREXIT', 'userexit_dir'],
    ['TIERREST', 'Not needed. SAS_lsm stops and then starts the tier.'],
  ], [4300, 5060]),
  Gap(),
  H2('Changes in Behavior'),
  Tbl([
    ['Area', 'SAS_lsm 4.x', 'SAS_lsm 5'],
    ['SSH direction', 'Described as required between all machines', 'Required from the controller to each machine only'],
    ['SSH setup', 'Manual', 'ssh-setup command; one password per host; FIPS-aware'],
    ['Per-tier values', 'A CONFIGDIR or INSTID value carried into the following tiers', 'Each tier uses its own value or the default'],
    ['First tier of a start', 'Always restarted', 'Restarted only if it is not already stopped'],
    ['Machines per tier', 'One', 'Any number, optionally in parallel'],
    ['UserExit success', 'Exit codes 0, 1, 2, and 4', 'Exit code 0 only'],
    ['UserExit file names', 'Lowercase only. POST scripts required SUCCESS or FAILURE.', 'Any case. POST scripts without SUCCESS or FAILURE run after either result.'],
    ['Version check', 'Contacted GitHub on every run', 'Only when you run sas_lsm version --check'],
    ['Wait between tiers', 'Fixed at 15 seconds', 'tier_delay, which can be set for each tier'],
    ['Log collection', 'Copied whole log files and trimmed them', 'Copies the end of recent log files, and creates a support bundle'],
    ['Exit codes', '0 or 1', '0 through 5'],
  ], [2200, 3480, 3680]),
  Gap(),

  // ------------------------------------------------------------ TROUBLESHOOTING
  H1('Troubleshooting'),
  Tbl([
    ['Symptom', 'Cause and Solution'],
    ['ssh-setup lists a host as [needs key]', 'This is expected the first time. Continue, and enter that account\'s password when prompted.'],
    ['ssh-setup lists a host as [problem]', 'The host name cannot be resolved, or the host or its SSH server cannot be reached. Correct the host name or network problem, and run ssh-setup again. Add --verbose to see the SSH program\'s own message.'],
    ['ssh-setup lists a host as [host key]', 'The host\'s SSH key has changed since SAS_lsm first connected to it. Confirm with your system administrator that the change is expected, run ssh-keygen -R host on the controller, and run ssh-setup again.'],
    ['ssh-setup summary shows a host as failed', 'The password was wrong, or the account cannot log on with a password. If the key was copied but logon still fails, check that ~/.ssh on that host has permission 700, ~/.ssh/authorized_keys has permission 600, and the home directory is not writable by the group.'],
    ['Ed25519 keys are not allowed in FIPS mode', 'The controller is in FIPS mode and ~/.ssh/id_ed25519 exists. Run sas_lsm ssh-setup to install an ECDSA key. After that, the warning is harmless; move id_ed25519 to another directory to stop it.'],
    ['ssh to sasinst@host failed (exit 255): Permission denied, during start, stop, or status', 'The SSH key is not installed for that account on that host. Run sas_lsm ssh-setup -c FILE.'],
    ['A tier is UNKNOWN, but the host can be reached', 'The status output did not contain the up or down text. Look at the full output of the status command in report.status, and correct up_pattern or down_pattern.'],
    ['sas.servers works when I log on, but not from SAS_lsm', 'SSH batch sessions do not read every profile file. Run ssh -o BatchMode=yes sasinst@host "/opt/sas/config/Lev1/sas.servers status" on the controller to reproduce the problem, and then set the missing variables in the account\'s ~/.bashrc.'],
    ['The middle tier fails to start, but it is running a few minutes later', 'SAS_lsm stopped waiting too soon. Increase retries for the middle tier. If sas.servers start itself takes longer than 15 minutes, also increase command_timeout.'],
    ['another SAS_lsm run (pid N) is active', 'Another action is running for the same configuration file (perhaps from cron). Wait for it to finish. If process N is not SAS_lsm, delete the .lock directory in the report directory.'],
    ['An lsf-grid tier does not start or stop', 'Run sas_lsm doctor. The passwordless sudo check must pass. You might also need to disable requiretty in the sudoers file.'],
    ['A command waits until command_timeout', 'A program started in the background kept the command open. Add >/dev/null 2>&1 & to background commands in custom tiers.'],
    ['ControlPath too long, or socket errors', 'Set ssh_multiplex: false in the settings section.'],
    ['tab characters are not allowed', 'Replace tab characters in the configuration file with spaces.'],
  ], [3300, 6060]),
  Gap(),
  P('If you need help, contact SAS Technical Support or open an issue on the SAS_lsm GitHub project page. Include the report file or the support bundle, with anything confidential removed.'),
];
