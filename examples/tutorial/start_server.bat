@echo off

if NOT EXIST run (
  mkdir run
  xcopy /e res\*.* run
)

cd run
..\..\bin\rufs.exe server
