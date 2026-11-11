#!/bin/bash
# End-to-end test for sas_lsm using simulated SAS hosts.
#
# A fake "ssh" placed first in PATH runs the remote command locally with
# FAKEHOST=<host>, so multi-host behaviour (ordering, quorum, failures,
# unreachable hosts) can be tested on one machine without real SSH.
#
# Usage: test/e2e.sh path/to/sas_lsm
set -u
BIN=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
W=$(mktemp -d "${TMPDIR:-/var/tmp}/sas_lsm_e2e.XXXXXX")
trap 'pkill -f "$W/lsf/etc/" 2>/dev/null; rm -rf "$W"' EXIT
pass=0; fail=0
ok()  { pass=$((pass+1)); echo "PASS  $*"; }
bad() { fail=$((fail+1)); echo "FAIL  $*"; }
expect_rc() { # want_rc description command...
  local want=$1 desc=$2; shift 2
  "$@" >"$W/out" 2>&1; local rc=$?
  if [ "$rc" = "$want" ]; then ok "$desc (rc $rc)"; else bad "$desc: rc $rc, want $want"; sed 's/^/      /' "$W/out" | tail -40; fi
}
expect_out() { # pattern description
  if grep -qE -- "$1" "$W/out"; then ok "$2"; else bad "$2: /$1/ not found"; sed 's/^/      /' "$W/out" | tail -40; fi
}
expect_no_out() { # pattern description
  if grep -qE -- "$1" "$W/out"; then bad "$2: /$1/ unexpectedly found"; sed 's/^/      /' "$W/out" | tail -40; else ok "$2"; fi
}

# ---------------------------------------------------------------- fixtures
mkdir -p "$W/bin" "$W/state" "$W/cfg/SASMeta/MetadataServer" "$W/cfg/Logs" "$W/ue" "$W/lsf/conf" "$W/lsf/etc"

cat >"$W/bin/ssh" <<EOF
#!/bin/sh
# fake ssh: record arguments, run the last argument with FAKEHOST set
echo "\$*" >> "$W/ssh.log"
host=""; cmd=""
for a in "\$@"; do host="\$cmd"; cmd="\$a"; done
case "\$host" in *unreachable*) echo "ssh: connect to host \$host port 22: No route to host" >&2; exit 255;; esac
FAKEHOST="\$host" exec /bin/sh -c "\$cmd"
EOF
chmod +x "$W/bin/ssh"
export PATH="$W/bin:$PATH"

# fake sas.servers: state per host; a host listed in state/failstart never comes up
cat >"$W/cfg/sas.servers" <<EOF
#!/bin/sh
h=\${FAKEHOST:-local}; f="$W/state/sas_\$h"
case "\$1" in
  start)  grep -qx "\$h" "$W/state/failstart" 2>/dev/null && { echo "ERROR: Object Spawner failed on \$h" >> "$W/cfg/Logs/ObjectSpawner_\$h.log"; exit 1; }
          touch "\$f"; echo "started \$h";;
  stop)   rm -f "\$f";;
  status) echo "SAS servers status:"
          if [ -f "\$f" ]; then echo "SAS Object Spawner 1 on \$h is UP"; else echo "SAS Object Spawner 1 on \$h is NOT up"; fi;;
esac
EOF
chmod +x "$W/cfg/sas.servers"

# fake MetadataServer.sh with stopCluster
cat >"$W/cfg/SASMeta/MetadataServer/MetadataServer.sh" <<EOF
#!/bin/sh
h=\${FAKEHOST:-local}
case "\$1" in
  start)       touch "$W/state/meta_\$h";;
  stop)        rm -f "$W/state/meta_\$h";;
  stopCluster) rm -f "$W/state"/meta_*;;
  status)      if [ -f "$W/state/meta_\$h" ]; then echo "SAS Metadata Server is RUNNING on \$h"; else echo "Server appears to be stopped"; fi;;
esac
EOF
chmod +x "$W/cfg/SASMeta/MetadataServer/MetadataServer.sh"

# fake Platform LSF: profile.lsf defines lsadmin/badmin that run dummy daemons
for d in lim res sbatchd mbatchd mbschd; do printf '#!/bin/sh\nwhile :; do sleep 1; done\n' >"$W/lsf/etc/$d"; chmod +x "$W/lsf/etc/$d"; done
cat >"$W/lsf/conf/profile.lsf" <<EOF
E="$W/lsf/etc"
bg() { nohup sh "\$E/\$1" >/dev/null 2>&1 & }
lsadmin() { case "\$1" in limstartup) bg lim; bg mbatchd; bg mbschd;; resstartup) bg res;;
  resshutdown) pkill -f "\$E/res\$";; limshutdown) pkill -f "\$E/lim\$"; pkill -f "\$E/mbatchd\$"; pkill -f "\$E/mbschd\$";; esac; }
badmin()  { case "\$1" in hstartup) bg sbatchd;; hshutdown) pkill -f "\$E/sbatchd\$";; esac; }
EOF

# userexits (4.x naming convention in a directory, plus config-listed scripts)
printf '#!/bin/sh\necho "ue legacy pre_stop on ${FAKEHOST:-local}" >> %s/ue.log\n' "$W" >"$W/ue/01_PRE_STOP_note.sh"
printf '#!/bin/sh\necho "ue legacy post_start on ${FAKEHOST:-local}" >> %s/ue.log\n' "$W" >"$W/ue/02_post_start_any.sh"
printf '#!/bin/sh\necho "ue config post_start_success on ${FAKEHOST:-local}" >> %s/ue.log\n' "$W" >"$W/ue/agent.sh"
printf '#!/bin/sh\nexit 3\n' >"$W/ue/fail.sh"
chmod +x "$W"/ue/*.sh

ME=$(id -un)
cat >"$W/site.yaml" <<EOF
version: 1
settings:
  status_root: $W/status
  ssh_multiplex: false
  log_minutes: 10
defaults:
  user: $ME
  configdir: $W/cfg
  retries: 3
  retry_interval: 1
  tier_delay: 0
  command_timeout: 30
tiers:
  - name: metadata
    type: metadata-cluster
    hosts: [meta1, meta2, meta3]
  - name: compute
    hosts: [localhost, compute2]
    parallel: true
    userexit_dir: $W/ue
    userexits:
      post_start_success:
        - run: $W/ue/agent.sh
          on: all
  - name: grid
    type: lsf-grid
    lsf:
      master: localhost
      envdir: $W/lsf/conf
      serverdir: $W/lsf/etc
      sudo: false
  - name: midtier
    hosts: [web1]
EOF
C="-c $W/site.yaml"

# ---------------------------------------------------------------- tests
expect_rc 0 "validate" "$BIN" validate $C
expect_out "4 tier\(s\)" "validate lists tiers"

expect_rc 0 "dry-run start" "$BIN" start $C --dry-run
expect_out "on every node meta1, meta2, meta3" "dry-run shows cluster start"
[ ! -e "$W/state/sas_web1" ] && ok "dry-run changed nothing" || bad "dry-run started something"

expect_rc 0 "start all tiers" "$BIN" start $C
expect_out "<<SUCCESS>> deployment tier services are operational" "start success message"
for f in meta_meta1 meta_meta2 meta_meta3 sas_local sas_compute2 sas_web1; do
  [ -e "$W/state/$f" ] && ok "running: $f" || bad "not running: $f"
done
pgrep -f "$W/lsf/etc/sbatchd" >/dev/null && ok "LSF daemons started" || bad "LSF daemons not started"
grep -q "ue config post_start_success on compute2" "$W/ue.log" && grep -q "ue config post_start_success on local" "$W/ue.log" \
  && ok "userexit with on: all ran on both hosts" || bad "userexit on: all"
grep -q "ue legacy post_start on" "$W/ue.log" && ok "legacy post_start script (no result) ran after success" || bad "legacy post_start"
grep -q -- "-l $ME compute2" "$W/ssh.log" && ok "ssh called as user@host for remote host" || bad "ssh args"
grep -q "localhost" "$W/ssh.log" && bad "localhost went through ssh" || ok "localhost ran without ssh"

expect_rc 0 "status after start" "$BIN" status $C
expect_out "deployment tier services are operational" "status operational"
expect_rc 0 "legacy flags: -s -c" "$BIN" -s -c "$W/site.yaml"

rm -f "$W/state/meta_meta3"
expect_rc 0 "status with 2 of 3 metadata nodes (quorum)" "$BIN" status $C
expect_out "quorum achieved \(2 of 3" "quorum note shown"
touch "$W/state/meta_meta3"

expect_rc 2 "start from 1 while later tiers are up -> dependency failure" "$BIN" start $C
expect_out "dependency check failed" "dependency message"

: > "$W/ue.log"
expect_rc 0 "stop down to tier 2 (by name)" "$BIN" stop $C --to compute
[ -e "$W/state/meta_meta1" ] && ok "tier 1 left running" || bad "tier 1 was stopped"
[ ! -e "$W/state/sas_web1" ] && [ ! -e "$W/state/sas_compute2" ] && ok "tiers 2-4 stopped" || bad "tiers 2-4 not stopped"
pgrep -f "$W/lsf/etc/" >/dev/null && bad "LSF daemons still running" || ok "LSF daemons stopped"
grep -q "ue legacy pre_stop" "$W/ue.log" && ok "upper-case 4.x userexit name matched" || bad "upper-case userexit not matched"

expect_rc 0 "status after partial stop (last action stop -> not a failure)" "$BIN" status $C
expect_out "NOT fully operational" "status reports partial deployment"

expect_rc 0 "start --from 2" "$BIN" start $C --from 2
expect_rc 0 "stop all (stopCluster path)" "$BIN" stop $C
ls "$W/state"/meta_* >/dev/null 2>&1 && bad "metadata nodes still running" || ok "metadata cluster stopped"
expect_rc 0 "status after full stop" "$BIN" status $C
expect_out "NOT operational" "status reports stopped"

# failure in the middle: compute2 never starts -> later tiers untouched, logs collected
echo compute2 > "$W/state/failstart"
expect_rc 1 "start with failing host" "$BIN" start $C --collect-logs
expect_out "tier 2:compute did not start" "failure names the tier"
[ ! -e "$W/state/sas_web1" ] && ok "later tiers were not started" || bad "later tier started after failure"
ls "$W/status/site"/support-bundle-*.tar.gz >/dev/null 2>&1 && ok "support bundle created" || bad "no support bundle"
grep -q "Object Spawner failed" "$W/status/site/logfiles.ANALYSIS" 2>/dev/null && ok "log analysis found the ERROR" || bad "log analysis"
rm -f "$W/state/failstart"
"$BIN" stop $C -q >/dev/null 2>&1

# required userexit failure aborts the run
sed -e "s#post_start_success:#pre_start:#" -e "s#run: $W/ue/agent.sh#run: $W/ue/fail.sh#" -e "s#on: all#required: true#" "$W/site.yaml" > "$W/req.yaml"
expect_rc 1 "required pre_start userexit failure stops the run" "$BIN" start -c "$W/req.yaml"
expect_out "required pre_start userexit failed" "required userexit message"
"$BIN" stop -c "$W/req.yaml" -q >/dev/null 2>&1

expect_rc 0 "userexits listing" "$BIN" userexits $C --tier compute
expect_out "pre_stop +$W/ue/01_PRE_STOP_note.sh" "listing shows legacy file for pre_stop"

# unreachable host
sed 's/hosts: \[web1\]/hosts: [web-unreachable]/' "$W/site.yaml" > "$W/unreach.yaml"
expect_rc 4 "status with unreachable host -> unknown" "$BIN" status -c "$W/unreach.yaml"
expect_out "ssh to .*web-unreachable failed" "ssh failure is reported"
expect_rc 1 "doctor finds the unreachable host" "$BIN" doctor -c "$W/unreach.yaml"
expect_out "FAIL\] connect without a password" "doctor connection failure"
expect_rc 0 "doctor on healthy config" "$BIN" doctor $C

# lock
mkdir -p "$W/status/site/.lock"; sleep 300 & echo $! > "$W/status/site/.lock/pid"
expect_rc 5 "concurrent run refused" "$BIN" status $C
kill %1 2>/dev/null; wait 2>/dev/null
expect_rc 0 "stale lock is cleared" "$BIN" status $C

# config errors
printf 'version: 1\ntiers:\n  - name: x\n    hosts: [h]\n    tpye: custom\n' > "$W/bad.yaml"
expect_rc 3 "invalid config rejected" "$BIN" validate -c "$W/bad.yaml"
expect_out 'line 5: .*unknown setting "tpye"' "typo reported with line number"

# convert from 4.x
cat > "$W/old.cfg" <<EOF
CONFIGDIR=/opt/sas/config/Lev1
INSTID=sas
MAXTIERS=6
SSHOPTIONS="-q -o StrictHostKeyChecking=no -o PasswordAuthentication=no"
MAXRETRY=10
SLEEP=6
STATUSROOT=/tmp
SASADMIN="a@example.com b@example.com"
for i in 1 2 3; do
TIERNAME[\$i]="Metadata node \$i"; TIERINST[\$i]=\${INSTID}; TIERHOST[\$i]=meta\$i
TIERSTAR[\$i]="\"nohup \${CONFIGDIR}/SASMeta/MetadataServer/MetadataServer.sh start >/dev/null 2>&1 &\""
TIERSTOP[\$i]="\${CONFIGDIR}/SASMeta/MetadataServer/MetadataServer.sh stopCluster"
TIERSTAT[\$i]="\${CONFIGDIR}/SASMeta/MetadataServer/MetadataServer.sh status 2>&1"
TIERSTAS[\$i]='Server appears to be stopped'; TIERSTOS[\$i]='is RUNNING on'; TIERLOGD[\$i]=\${CONFIGDIR}; TIERTYPE[\$i]='MDCN'
done
TIERNAME[4]='Metadata cluster validation (MDCV)'; TIERINST[4]=\${TIERINST[1]}; TIERHOST[4]=\${TIERHOST[1]}; TIERTYPE[4]='MDCV'
TIERNAME[5]='Compute'; TIERINST[5]=\${INSTID}; TIERHOST[5]=compute1
TIERSTAR[5]="\${CONFIGDIR}/sas.servers start"; TIERSTOP[5]="\${CONFIGDIR}/sas.servers stop"; TIERSTAT[5]="\${CONFIGDIR}/sas.servers status"
TIERSTAS[5]='is NOT up'; TIERSTOS[5]='is UP'; TIERLOGD[5]=\${CONFIGDIR}; TIERTYPE[5]='standard'; TIEREXIT[5]=/opt/sas/ue
CONFIGDIR=/opt/sas/configmid/Lev1
TIERNAME[6]='Mid Tier'; TIERINST[6]=sasmid; TIERHOST[6]=web1
TIERSTAR[6]="\${CONFIGDIR}/sas.servers start"; TIERSTOP[6]="\${CONFIGDIR}/sas.servers stop"; TIERSTAT[6]="\${CONFIGDIR}/sas.servers status"
TIERSTAS[6]='is NOT up'; TIERSTOS[6]='is UP'; TIERLOGD[6]=\${CONFIGDIR}; TIERTYPE[6]='standard'
EOF
"$BIN" convert "$W/old.cfg" > "$W/converted.yaml" 2>"$W/notes"; rc=$?
[ $rc = 0 ] && ok "convert 4.x config" || { bad "convert rc $rc"; cat "$W/notes"; }
cp "$W/converted.yaml" "$W/out"
expect_out "type: metadata-cluster" "cluster tiers merged"
expect_out "hosts: \[meta1, meta2, meta3\]" "cluster hosts"
expect_out "configdir: /opt/sas/configmid/Lev1" "per-tier configdir kept"
expect_out "user: sasmid" "per-tier user kept"
expect_out "name: mid-tier" "tier name made easy to type"
expect_out "StrictHostKeyChecking=accept-new" "insecure host key option replaced"
expect_rc 0 "converted config validates" "$BIN" validate -c "$W/converted.yaml"

# discover
mkdir -p "$W/d/meta/SASMeta/MetadataServer" "$W/d/web/Web/WebAppServer/SASServer1_1"
cat > "$W/bin/ssh" <<EOF
#!/bin/sh
host=""; cmd=""
for a in "\$@"; do host="\$cmd"; cmd="\$a"; done
case "\$host" in meta*) d=$W/d/meta ;; web*) d=$W/d/web ;; *) d=$W/d/compute ;; esac
mkdir -p "\$d"; touch "\$d/sas.servers"; chmod +x "\$d/sas.servers"
[ "\${host#meta}" != "\$host" ] && { touch "\$d/SASMeta/MetadataServer/MetadataServer.sh"; chmod +x "\$d/SASMeta/MetadataServer/MetadataServer.sh"; }
exec /bin/sh -c "\$(echo "\$cmd" | sed "s#/opt/sas/config/Lev1#\$d#")"
EOF
"$BIN" discover --hosts meta1,compute1,compute2,web1 --configdir /opt/sas/config/Lev1 --user sas > "$W/disc.yaml" 2>/dev/null; rc=$?
[ $rc = 0 ] && ok "discover" || bad "discover rc $rc"
cp "$W/disc.yaml" "$W/out"
grep -A2 "name: metadata" "$W/out" | grep -q "hosts: \[meta1\]" && ok "discover puts metadata first" || bad "discover metadata"
grep -A2 "name: compute" "$W/out" | grep -q "hosts: \[compute1, compute2\]" && ok "discover groups compute hosts" || bad "discover compute group"
grep -A2 "name: midtier" "$W/out" | grep -q "hosts: \[web1\]" && ok "discover finds middle tier" || bad "discover midtier"
expect_rc 0 "discovered config validates" "$BIN" validate -c "$W/disc.yaml"

# ---------------------------------------------------------------- ssh-setup
# Fake ssh-keygen, ssh-copy-id and ssh. A host accepts the key once
# ssh-copy-id has "installed" it (a marker file). HOME is redirected so the
# real ~/.ssh is never touched.
export HOME="$W/home"; mkdir -p "$HOME"
cat > "$W/bin/ssh-keygen" <<EOF
#!/bin/sh
f=""; t=""
while [ \$# -gt 0 ]; do case "\$1" in -f) f="\$2"; shift;; -t) t="\$2"; shift;; esac; shift; done
echo "PRIVATE \$t" > "\$f"; echo "ssh-\$t AAAAfake sas_lsm" > "\$f.pub"
EOF
cat > "$W/bin/ssh-copy-id" <<EOF
#!/bin/sh
for a in "\$@"; do last="\$a"; done
host="\${last#*@}"
case "\$host" in badpw*) echo "Permission denied, please try again." >&2; exit 1;; esac
mkdir -p "$W/keys"; touch "$W/keys/\$host"; echo "Number of key(s) added: 1"
EOF
cat > "$W/bin/ssh" <<EOF
#!/bin/sh
host=""; cmd=""
for a in "\$@"; do host="\$cmd"; cmd="\$a"; done
case "\$host" in
  nosuch*)  echo "ssh: Could not resolve hostname \$host: Name or service not known" >&2; exit 255;;
  changed*) echo "@@@ WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! @@@" >&2; echo "Host key verification failed." >&2; exit 255;;
esac
if [ ! -f "$W/keys/\$host" ]; then
  echo "sshkey_read: Ed25519 keys are not allowed in FIPS mode" >&2
  echo "$ME@\$host: Permission denied (publickey,gssapi-keyex,gssapi-with-mic,password)." >&2; exit 255
fi
exec /bin/sh -c "\$cmd"
EOF
chmod +x "$W/bin/ssh-keygen" "$W/bin/ssh-copy-id" "$W/bin/ssh"
mkdir -p "$W/keys"; touch "$W/keys/ready1"

expect_rc 0 "ssh-setup --hosts (one ready, two need the key)" "$BIN" ssh-setup --hosts ready1,new1,new2 --yes
expect_out "\[ready\] +$ME@ready1 +key already installed" "ready host shown as [ready]"
expect_out "\[needs key\] +$ME@new1 +key not installed yet \(expected\)" "new host shown as [needs key] (expected)"
expect_no_out "Permission denied|FIPS mode|authentication failed" "raw ssh errors are not shown for the expected case"
expect_out "2 hosts need the key. For each one, ssh-copy-id will ask for that account's" "next step is explained"
expect_out "==> \[1/2\] $ME@new1: enter the password for $ME" "password prompt names host and account"
expect_out "ready: +3 host\(s\) \(1 already set up, 2 installed now\)" "summary counts"
[ -f "$HOME/.ssh/id_ed25519.pub" ] && ok "key created (non-FIPS: ed25519)" || bad "no key created"

expect_rc 0 "ssh-setup again: nothing to do" "$BIN" ssh-setup --hosts ready1,new1,new2
expect_out "All hosts are ready. No passwords are needed." "rerun needs no passwords"

expect_rc 1 "ssh-setup with unreachable and changed-host-key hosts" "$BIN" ssh-setup --hosts new3,nosuchhost,changedhost --yes
expect_out "\[problem\] +$ME@nosuchhost +host name could not be resolved" "DNS failure shown as [problem]"
expect_out "\[host key\] +$ME@changedhost +host key has changed" "host key change shown as [host key]"
expect_out "2 host\(s\) have a problem that a password will not fix" "problems are explained"
expect_out "ssh-keygen -R changedhost" "host key fix suggested"
expect_out "1 host needs the key" "only the fixable host is offered"
expect_out "skipped: +2 host\(s\)" "problem hosts reported as skipped"
[ -f "$W/keys/new3" ] && ok "fixable host still installed" || bad "fixable host not installed"
[ ! -f "$W/keys/nosuchhost" ] && ok "no password asked for unreachable host" || bad "ssh-copy-id ran for unreachable host"

expect_rc 1 "ssh-setup with a wrong password" "$BIN" ssh-setup --hosts badpw1 --yes
expect_out "failed: +1 host\(s\)" "wrong password reported in summary"

expect_rc 0 "ssh-setup --verbose shows ssh's own message" "$BIN" ssh-setup --hosts new4 --yes --verbose
expect_out "ssh: .*Permission denied" "verbose shows raw ssh output"

expect_rc 1 "ssh-setup --key-type ed25519 rejects bad type" "$BIN" ssh-setup --hosts new5 --key-type dsa
expect_rc 0 "ssh-setup with only the local host" "$BIN" ssh-setup --hosts localhost
expect_out "no SSH setup is needed" "local-only setup skipped"

echo
echo "e2e: $pass passed, $fail failed"
[ $fail -eq 0 ]
