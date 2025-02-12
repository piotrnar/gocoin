@echo off
:again
client.exe
if errorlevel 67 goto terminate
if errorlevel 66 goto again
:terminate
