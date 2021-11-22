# SAS_lsm_Grid.sh
# Programmer: Greg Arledge, SAS Technical Support
# Date: 11/22/2021

# Utility to start, stop, and check the status of the SAS Grid
# Meant for use with SAS_lsm script but can also be run standalone
# as at grid control mechanism.

# The account used to run the commands will require passwordless
# sudo priveleges to be able to start grid services. Some distros
# may also require removal of the !requiretty flag in /etc/sudoers,
# do this if you see an error about requiring a tty to run sudo.

##############################################################
# Configuration Section (Should be edited by Administrator)
##############################################################

# Full path to LSF commands
#LSFHOME=/opt/sas/lsf/10.1/linux2.6-glibc2.3-x86_64/bin
LSFHOME=

# Full path to LSF processes (LSF_SERVERDIR)
#LSFPROC=/opt/sas/lsf/10.1/linux2.6-glibc2.3-x86_64/etc
LSFPROC=

# Full path to LSF conf (LSF_ENVDIR)
#LSFCONF=/opt/sas/lsf/conf
LSFCONF=

# Full path to PM commands 
#PMHOME=/opt/sas/pm/10.2/bin
PMHOME=

# Full path to PM processes (JS_SERVERDIR)
#PMPROC=/opt/sas/pm/10.2/linux2.6-glibc2.3-x86_64/etc
PMPROC=

# Full path to PM conf (JS_ENVDIR)
#PMCONF=/opt/sas/pm/conf
PMCONF=

# Hostname of machine that hosts Process Manager (PM)
#PMHOST=processmanager.examplehost.com
PMHOST=

# Full path to GMS commands (LSF_TOP/gms/bin)
#GMSHOME=/opt/sas/gms/bin
GMSHOME=

# Full path to GMS processes (LSF_TOP/gms/etc)
#GMSPROC=/opt/sas/gms/etc
GMSPROC=

# Hostname of machine that hosts Grid Monitoring Services (GMS)
#GMSHOST=gridmon.examplehost.com
GMSHOST=

# Hostname of the master LSF node
#MASTER=lsfmaster.examplehost.com
MASTER=

# List of LSF node hostnames that are not the master node
#NODEHOSTS="lsfnode1.examplehost.com lsfnode2.examplehost.com lsfnode3.examplehost.com"
NODEHOSTS=

# Account used to run commands (must be able to sudo without password prompting)
#USERID=sas
USERID=

# Is EGO Installed? If yes, then set EGO to enabled and specify appropriate host/user/pass. If no, set EGO to disabled.
#EGO="enabled"
#EGOHOST=ego.examplehost.com
#EGOUSER="Admin"
#EGOPASS="Admin"
EGO="disabled"
EGOHOST=
EGOUSER=
EGOPASS=

# Is EGO configured to start RES and SBATCHD? If yes, then set EGOCONTROL to enabled. If no, set EGOCONTROL to disabled.
#EGOCONTROL="disabled"
EGOCONTROL=

# Is EGO Process Monitor Failover enabled?  If yes, uncomment PMFAILHOST variable and set to the appropriate hostname.
#PMFAILHOST=egoprocmon.examplehost.com
#PMFAILHOST=

# Interval of time to wait between starts and stop actions, in seconds. Default: 10 seconds.
#SLEEPTIME=10
SLEEPTIME=10


##############################################################
# End Configuration Section
##############################################################





##################################################################################################
##################################################################################################
##################################################################################################

####################################### CODE SECTION #############################################
#######################################  DO NOT EDIT #############################################

##################################################################################################
##################################################################################################
##################################################################################################

STARTGCMD=start
STOPGCMD=stop
STATUSGCMD=status

# Begin Start Function

# Start the lim res and sbatch grid services on the nodes
# first making sure that the master node is first, then
# ssh to GMS host and start gaadmin, then ssh to PM host
# and start jadmin.

start_grid()
{

# Declare and array using user defined node host variable
declare -a GRIDNODES=(${NODEHOSTS})

# Master Node LSF Daemon Startup
ssh ${USERID}@${MASTER} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin limstartup'"
sleep ${SLEEPTIME}
if [ ${EGOCONTROL}="disabled" ]
  then ssh ${USERID}@${MASTER} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin resstartup'"
       sleep ${SLEEPTIME}
       ssh ${USERID}@${MASTER} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/badmin hstartup'"
       sleep ${SLEEPTIME}
fi

# Slave Nodes LSF Daemon Startup
# Loop through GRIDNODES array
for i in "${!GRIDNODES[@]}"
  do
    ssh ${USERID}@${GRIDNODES[$i]} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin limstartup'"
    sleep ${SLEEPTIME}
    if [ ${EGOCONTROL}="disabled" ]
      then ssh ${USERID}@${GRIDNODES[$i]} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin resstartup'"
           sleep ${SLEEPTIME}
           ssh ${USERID}@${GRIDNODES[$i]} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/badmin hstartup'"
           sleep ${SLEEPTIME}
    fi
  done

# Start Grid Monitoring Service on GMS Host
#ssh ${USERID}@${GMSHOST} "sudo ${GMSHOME}/gaadmin start"
#sleep ${SLEEPTIME}

# Start Process Manager on PM Host
if [ ${EGO}="enabled" ]
  then EGOCMD="egosh user logon -u ${EGOUSER} -x ${EGOPASS}"
       EGOLIST=`ssh ${USER}@${EGOHOST} sh -c "'. ${LSFCONF}/profile.lsf && ${EGOCMD} && egosh service list -l'"`
       echo ${EGOLIST};
       case "${EGOLIST}" in 
         *ProcessManager*)
            ssh ${USER}@${EGOHOST} ${EGOCMD} && egosh service start ProcessManager
            ;;
         *)
            ssh ${USERID}@${PMHOST} sudo -s sh -c "'. ${PMCONF}/profile.js && ${PMHOME}/jadmin start'"
            ;;
       esac
  else ssh ${USERID}@${PMHOST} sudo -s sh -c "'. ${PMCONF}/profile.js && ${PMHOME}/jadmin start'"
fi
sleep ${SLEEPTIME}

}
# End of Start Function

# Begin Stop Function

# Stop jadmin on PM host, then stop gaadmin on GMS host,
# then stop sbatch, res, and lim on all nodes making sure
# to stop the master node last

stop_grid()
{

# Declare and array using user defined node host variable
declare -a GRIDNODES=(${NODEHOSTS})

# Stop Process Manager on PM Host
if [ ${EGO}="enabled" ]
  then EGOCMD="egosh user logon -u ${EGOUSER} -x ${EGOPASS}"
       EGOLIST=`ssh ${USER}@${EGOHOST} sh -c "'. ${LSFCONF}/profile.lsf && ${EGOCMD} && egosh service list -l'"`
       echo ${EGOLIST};
       case "${EGOLIST}" in
         *ProcessManager*)
            ssh ${USER}@${EGOHOST} ${EGOCMD} && egosh service stop ProcessManager
            ;;
         *)
            ssh ${USERID}@${PMHOST} sudo -s sh -c "'. ${PMCONF}/profile.js && ${PMHOME}/jadmin stop'"
            ;;
       esac
  else ssh ${USERID}@${PMHOST} sudo -s sh -c "'. ${PMCONF}/profile.js && ${PMHOME}/jadmin stop'"
fi
sleep ${SLEEPTIME}


# Stop Grid Monitoring Service on GMS Host
#ssh ${USERID}@${GMSHOST} "sudo ${GMSHOME}/gaadmin stop"
#sleep ${SLEEPTIME}

# Loop through GRIDNODES array and stop LSF
# services on all slave nodes
for i in "${!GRIDNODES[@]}"
  do
    ssh ${USERID}@${GRIDNODES[$i]} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/badmin hshutdown'"
    sleep ${SLEEPTIME}
    ssh ${USERID}@${GRIDNODES[$i]} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin resshutdown'"
    sleep ${SLEEPTIME}
    ssh ${USERID}@${GRIDNODES[$i]} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin limshutdown'"
    sleep ${SLEEPTIME}
  done

# Stop LSF Services on Master Node
ssh ${USERID}@${MASTER} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/badmin hshutdown'"
sleep ${SLEEPTIME}
ssh ${USERID}@${MASTER} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin resshutdown'"
sleep ${SLEEPTIME}
ssh ${USERID}@${MASTER} sudo -s sh -c "'. ${LSFCONF}/profile.lsf && ${LSFHOME}/lsadmin limshutdown'"
sleep ${SLEEPTIME}

}
# End Stop Function

# Begin Status Function

# Check the status of LSF services on all nodes,
# gaadmin process on the GMS host and jadmin
# process on the PM host

grid_status()
{

declare -a GRIDNODES=(${NODEHOSTS})

for i in "${!GRIDNODES[@]}"
  do
    echo ""
    echo "Status of ${GRIDNODES[$i]} Grid/LSF Services:"
    LIMCMD="ps -ef | grep ${LSFPROC}/lim | grep -v grep | wc -l"
    LIMTOKEN=`ssh ${USERID}@${GRIDNODES[$i]} ${LIMCMD}`
    if [ ${LIMTOKEN} -gt 0 ]
      then echo "    Load Information Manager is UP."
      else echo "    Load Infromation Manager is NOT Up."
    fi
    RESCMD="ps -ef | grep ${LSFPROC}/res$ | grep -v grep | wc -l"
    RESTOKEN=`ssh ${USERID}@${GRIDNODES[$i]} ${RESCMD}`
    if [ ${RESTOKEN} -gt 0 ]
      then echo "    Remote Execution Server is UP."
      else echo "    Remote Exectuion Server is NOT Up."
    fi
    SBATCHDCMD="ps -ef | grep ${LSFPROC}/sbatchd | grep -v grep | wc -l"
    SBATCHDTOKEN=`ssh ${USERID}@${GRIDNODES[$i]} ${SBATCHDCMD}`
    if [ ${SBATCHDTOKEN} -gt 0 ]
      then echo "    Slave Batch Daemon is UP."
      else echo "    Slave Batch Daemon is NOT Up."
    fi
    PIMCMD="ps -ef | grep ${LSFPROC}/pim | grep -v grep | wc -l"
    PIMTOKEN=`ssh ${USERID}@${GRIDNODES[$i]} ${PIMCMD}`
    if [ ${PIMTOKEN} -gt 0 ]
      then echo "    Process Information Manager Daemon is UP."
      else echo "    Process Information Manager Daemon is NOT Up."
    fi
    if [ ${EGO}="enabled" ]
      then PEMCMD="ps -ef | grep ${LSFPROC}/pem | grep -v grep | wc -l"
           PIMTOKEN=`ssh ${USERID}@${GRIDNODES[$i]} ${PEMCMD}`
           if [ ${PIMTOKEN} -gt 0 ]
             then echo "    Process Execution Manager is UP."
             else echo "    Process Execution Manager is NOT Up."
           fi
    fi
  done
echo ""
echo "Status of ${MASTER} (Master Node) Grid/LSF Services:"
LIMCMD="ps -ef | grep ${LSFPROC}/lim | grep -v grep | wc -l"
LIMTOKEN=`ssh ${USERID}@${MASTER} ${LIMCMD}`
  if [ ${LIMTOKEN} -gt 0 ]
    then echo "    Load Information Manager is UP."
    else echo "    Load Infromation Manager is NOT Up."
  fi
RESCMD="ps -ef | grep ${LSFPROC}/res$ | grep -v grep | wc -l"
RESTOKEN=`ssh ${USERID}@${MASTER} ${RESCMD}`
  if [ ${RESTOKEN} -gt 0 ]
    then echo "    Remote Execution Server is UP."
    else echo "    Remote Exectuion Server is NOT Up."
  fi
SBATCHDCMD="ps -ef | grep ${LSFPROC}/sbatchd | grep -v grep | wc -l"
SBATCHDTOKEN=`ssh ${USERID}@${MASTER} ${SBATCHDCMD}`
  if [ ${SBATCHDTOKEN} -gt 0 ]
    then echo "    Slave Batch Daemon is UP."
    else echo "    Slave Batch Daemon is NOT Up."
  fi
PIMCMD="ps -ef | grep ${LSFPROC}/pim | grep -v grep | wc -l"
PIMTOKEN=`ssh ${USERID}@${MASTER} ${PIMCMD}`
  if [ ${PIMTOKEN} -gt 0 ]
    then echo "    Process Information Manager Daemon is UP."
    else echo "    Process Information Manager Daemon is NOT Up."
  fi
MBATCHDCMD="ps -ef | grep ${LSFPROC}/mbatchd | grep -v grep | wc -l"
MBATCHDTOKEN=`ssh ${USERID}@${MASTER} ${MBATCHDCMD}`
  if [ ${MBATCHDTOKEN} -gt 0 ]
    then echo "    Master Batch Daemon is UP."
    else echo "    Master Batch Daemon is NOT Up."
  fi
MBSCHDCMD="ps -ef | grep ${LSFPROC}/mbschd | grep -v grep | wc -l"
MBSCHDTOKEN=`ssh ${USERID}@${MASTER} ${MBSCHDCMD}`
  if [ ${MBSCHDTOKEN} -gt 0 ]
    then echo "    Master Batch Scheduler Daemon is UP."
    else echo "    Master Batch Scheduler Daemon is NOT Up."
  fi
if [ ${EGO}="enabled" ]
  then PEMCMD="ps -ef | grep ${LSFPROC}/pem | grep -v grep | wc -l"
       PIMTOKEN=`ssh ${USERID}@${MASTER} ${PEMCMD}`
         if [ ${PIMTOKEN} -gt 0 ]
           then echo "    Process Execution Manager is UP."
           else echo "    Process Execution Manager is NOT Up."
         fi
       echo ""
       echo "Status of EGO Services: (${EGOHOST}):"
       VEMKDCMD="ps -ef | grep ${LSFPROC}/vemkd | grep -v grep | wc -l"
       VEMKDTOKEN=`ssh ${USERID}@${EGOHOST} ${VEMKDCMD}`
         if [ ${VEMKDTOKEN} -gt 0 ]
           then echo "    Virtual Execution Manager is UP."
           else echo "    Virtual Execution Manager is NOT Up."
         fi
       EGOSCCMD="ps -ef | grep ${LSFPROC}/egosc | grep -v grep | wc -l"
       EGOSCTOKEN=`ssh ${USERID}@${EGOHOST} ${EGOSCCMD}`
         if [ ${EGOSCTOKEN} -gt 0 ]
           then echo "    EGO Service Controller is UP."
           else echo "    EGO Service Controller is NOT Up."
         fi
fi
echo ""
echo "Status of Grid Management Service (${GMSHOST}):"
GMSCMD="ps -ef | grep ${GMSPROC}/gabd | grep -v grep | wc -l"
GMSTOKEN=`ssh ${USERID}@${GMSHOST} ${GMSCMD}`
  if [ ${GMSTOKEN} -gt 0 ]
    then echo "    Grid Management Service is UP."
    else echo "    Grid Management Service is NOT Up."
  fi
echo ""

echo "Status of the Process Manager (${PMHOST}):"
PMCMD="ps -ef | grep ${PMPROC}/jfd | grep -v grep | wc -l"
PMTOKEN=`ssh ${USERID}@${PMHOST} sudo ${PMCMD}`
if [[ -v PMFAILHOST ]]
  then PMFAILTOKEN=`ssh ${USERID}@${PMFAILHOST} sudo ${PMCMD}`
fi
  if [ ${PMTOKEN} -gt 0 ]
    then echo "    Process Manager is UP."
    else if [[ -v PMFAILTOKEN ]]
           then echo "    Process Manager not running on primary Process Manager Host.  Checking Process Manager Failover Host."
                if [ ${PMFAILTOKEN} -gt 0 ]
                  then echo "    Process Manager is UP."
                  else echo "    Process Manager is NOT Up."
                fi
           else echo "    Process Manager is NOT Up."
         fi     
  fi
echo ""

}


case "$1" in
  start)
    start_grid;
    exit $?
    ;;
  stop)
    stop_grid;
    exit $?
    ;;
  status)
    grid_status;
    exit $?
    ;;
  *)
    echo "Usage $0 {$STARTGCMD|$STOPGCMD|$STATUSGCMD}"
    exit 1;
esac

