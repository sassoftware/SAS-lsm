Master script file is built from combining all files in the subdirectory "lsm_components".
The file in the root directory "SAS_lsm" is the built master script file that can be used or sent to customers.

//TODO

1) Better documentation?

4.0 Documentation

Automated Testing
jenkins

Sample UserExits -
Deployment Agents

systemd sasnote

get it working with Viya (vs Viya Arc)

survey + milestone

sas_lsm in Sirius as a Product/Topic

version command
* This might not be the best way, but it works: `curl -v --silent http://support.sas.com/kb/58/231.html 2>&1 | grep "The current release" | sed 's/[^0-9.]*//g' | sed 's/\.$//'`
  * Returns "3.0" currently.
  * Basically looks for the line where we say "The current release is..." and does the following with it:
    * Echoes to stdout,
    * Pass to sed to keep only numerics and decimal-points,
    * Pass to sed again to strip the period at the end of the output (we don't care about that one, it's not part of the version number)


tracking Working time + Deadlines

changeDetector