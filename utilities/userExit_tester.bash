#!/bin/bash
#userExit Management Tester script
#derived from userExit_management function of SAS_lsm
# v. 1.0.0 (based on SAS_lsm 4.0.0 source) by Andy Foreman        

        TIEX=$1 #location of UserExit scripts
        UETYPE=$2  #pre or post
        ACTION=$3 #start or stop
	UESTAT=$4  #success or failure
        declare -a SCRIPTLIST
        echo ""

        echo "The specified directory ${TIEX} contains the following files:"
        echo ""
        ls -la ${TIEX}
        echo ""


	if [ -z ${UESTAT} ] # Will run in pre mode even without UESTAT.  Pre cannot contain success or failure because nothing has happened yet.
	  then
            while IFS=  read -r -d $'\0';  # While loop to read results of the find command into SCRIPTLIST array 
	      do
            SCRIPTLIST+=("$REPLY")
        done < <(find ${TIEX} -name "*_${UETYPE}_${ACTION}_*" -print0 | sort -z) ######NEED TO SEE IF SORT IS ON SOLARIS#####
        else           
            while IFS=  read -r -d $'\0';  # While loop to read results of the find command into SCRIPTLIST array 
	      do
            SCRIPTLIST+=("$REPLY")
        done < <(find ${TIEX} -name "*_${UETYPE}_${ACTION}_${UESTAT}_*" -print0 | sort -z)
	fi
        #echo "List: ${SCRIPTLIST[*]}"
	if [ ${#SCRIPTLIST[@]} -eq 0 ]
          then echo "No UserExit Scripts Found in directory ${TIEX} for UserExit action ${UETYPE} ${ACTION} ${UESTAT}"
               echo "Please verify that the expected files are in the directory contents shown above, and that the specified action type exists."
               echo ""
          else echo "Matching UserExit Scripts were located in directory ${TIEX} for UserExit action ${UETYPE} ${ACTION} ${UESTAT}"
               echo "The following UserExit scripts will be run in the displayed order when the specified action condition is met:"
               echo ""
               for i in "${!SCRIPTLIST[@]}"
	         do
                   echo "$(( $i+1 )).   ${SCRIPTLIST[$i]}"
	         done
               echo ""
        fi
	unset SCRIPTLIST
