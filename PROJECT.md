# Powermeter

Powermeter is a simple Go CLI tool. It wraps the macOS `powermetrics` utility. It measures how much power the processes on the current computer use.

The application has a menu with these options:
- Start measure test
- Change measure setup
- History of measures
- About section
- Exit

When a measure starts, a loading bar fills continuously during the elapsed time. During this period, the user can do usual work and then return to see the results.
The result contains the top 20 processes by power consumption (how much power they use and how much CPU time acquire).
Per-process power is shown in milliwatts (mW). It is an estimate: the system power is split between processes in proportion to their energy impact.
The user can navigate the table. If the user presses Enter, the browser opens a Google query "Why <process> consumes too much power on my mac?".
The user can return to the menu to start another test or open other sections.