@echo off
:again
client.exe
if exist .restart goto again
