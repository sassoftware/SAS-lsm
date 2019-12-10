Master script file is built from combining all files in the subdirectory "lsm_components".
The file in the root directory "SAS_lsm" is the built master script file that can be used or sent to customers.

//TODO
1) Design a script to automatically generate the master script file by using all the files in "lsm_components". The contents are:
*     "header.txt" (comments at top of script)
*     various "XYZ.fn" files (one for each function)
*     "main.main" (main calls at bottom of script which are not in a function)

    
Given this, we can easily design a script to find and copy these files into a combined single file in the preferred order (likely as listed above) which would then be the master script file.

2) Should probably put doItYourself.sh config-file generator script on here at some point, it's fully working

3) Better documentation?