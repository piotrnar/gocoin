@echo off
:again
client.exe %1 %2 %3 %4 %5 %6 %7 %8 %9
if errorlevel 67 goto terminate
if errorlevel 66 goto again
:terminate
