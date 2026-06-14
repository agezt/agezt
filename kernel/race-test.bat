@echo off
set CGO_ENABLED=1
cd /d D:\Codebox\PROJECTS\AGEZT\kernel
go test -race ./controlplane/... -count=1 -timeout=120s
