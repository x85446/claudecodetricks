Demo on skill-builder:

/skill-builder I need a skill for a common task I do.  I typically launch incus machines on remote
  machines either
    by switching incus remote locally, or by sshing to a known host with that incus configured.  from there I will
    launch a machine, using defaults typically.  sometimes i may specify cpu <count> and mem <count> and disk <count>
    for hard drive size.  I will always need to give you a name, so if I fail to give you name, ask. but if I don't
    give you disk size, ram or cpu then just assume the default is recorded.  also I will tell you --vm or make a vm,
    to overide the default of container.  After you launch said machine,  I expect you to run 'incusmagic ssh enable
  <name> travis, which will install ssh and my keys to get into that machine.  From there you almost always need to
  caputre the ip and update / insert into my local hosts ~/.ssh/config.d/<file> to enable me to ssh to that machine.
  there needs to be a small memory file that the skill references to understand what machine belongs there.  Anything
  dealing with fieldstone or bryan, belongs in the fieldstone file.  Aything dealing with "ed" or "eds house" or
  backbay, houston" belongs in ed-house.  Anything local or at my house, belongs in cypress. Anything at datafoundry
  or switch or datacenter, belongs in df-austin.  Moreover, if specifiy the machine H-series, P-series, H91...H98
  P91...P98 that goes to df-austin, if I specify fieldstone that is bryan. If I specify polaris or ranger or echo or
  explorer, that is ed-house.  Inside those files you will find a pattern for how I remote to machines.  typically
  they need a proxyjump, exept for df-austin.  follow the pattern, keep the config.d/<files> alphabetical, by host.
Somtimes I will ask you do this from a remote machine.  you are to update my host by "ssh cypressMini" in most cases
will work.  inform if not by simply pasting out the text that goes in the file and piping to pbcopy.  Inform me what
file I need to edit.


incus launch command already has defaults for all the machines we run.  I forogot to tell you two things.  1. I may
   ask you to delete previous machines, or snapshot a machine, or figure out the settings of a machine, or figure out
   the configuration of machine as well, so specialize on those tasks too.  Second i want you default to ubuntu/24.04
   unless I tell you otherwise in the freeflowing text.  in the helper text just mention /incus <name> [cpu,mem,disk,OS]
    <what to do>


xs