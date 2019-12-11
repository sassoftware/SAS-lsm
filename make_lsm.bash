#!/bin/bash
#
# MAKE LSM FILE
#
# Version 1.0: 11 December 2019. Produced by Andy Foreman.
##########
# This is a development utility to build the SAS_lsm master script file.
# Not intended for use by end-customers. It can be safely ignored or deleted.
#
# USAGE:
# 
#   1) Place this file in the root directory of the SAS_lsm package (the place where the master file should be generated)
#   2) Ensure a subdirectory "lsm_components" containing desired components exists under this script's executing directory and is readable
#   3) Run this script, the output file will be saved in the current working directory (make sure you can write there)
#        - You can specify an output filename directly in the command call, i.e. `./make_lsm.bash myfilename`
#        - If you do not specify an output filename in the command call, you will be prompted to enter one before the output file is generated
#   4) THE OUTPUT FILENAME WILL BE OVERWRITTEN!
#
# NOTES:
#
#  - As a development utility, there is no input-validation or error-checking here.
#
#  - The "lsm_components" subdirectory will be searched in the following order:
#      1) Any .txt files (expected to be comments at top of output script)
#      2) Any .fn files (expected to be the individual script functions of SAS_lsm)
#      3) The file "main.main" (expected to contain main function calls and anything else that cannot go inside a function)
#
#  - The output script will be written out in the order of files searched. Logically this means you get .txt files (comments) at the top, the individual .fn files (functions) in any order, and then the main.main file (main instructions) at the bottom.
##########

pwd="$(pwd)"
dirname="$(dirname $0)"
outfile=$1

if [ -z "$outfile" ] #if user did not specify outfile name in command, ask for it
then
read -e -p "Specify output file name: " outfile
fi

echo -n > $pwd/$outfile  #delete outfile contents, if exists

find $dirname/lsm_components/ | grep .*.txt | while read filename; do
   cat $filename >> $pwd/$outfile 
done

find $dirname/lsm_components/ | grep .*.fn | while read filename; do  #loop through each filename in defined path, and subdirs technically
   cat $filename >> $pwd/$outfile
done

find $dirname/lsm_components/ | grep main.main | while read filename; do
   cat $filename >> $pwd/$outfile
done

echo "File has been built and saved at path: $pwd/$outfile"
