@echo off
go build next_block.go
if errorlevel 1 goto fin
go build block_validator.go
if errorlevel 1 goto fin
echo While the validator is running feed it with the test blocks by executing: java -jar bitcoin.jar
block_validator.exe
:fin
