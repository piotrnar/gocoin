@echo off
go build next_block.go
if errorlevel 1 goto fin
pskill block_validator
go build block_validator.go
if errorlevel 1 goto fin
start block_validator
java -jar bitcoin.jar
:fin
del next_block.exe block_validator.exe dupa.bin
