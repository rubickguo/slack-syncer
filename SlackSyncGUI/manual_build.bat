@echo off
set "PATH=%PATH%;C:\Program Files\Go\bin"
echo Building frontend...
cd frontend
call npm run build
if %errorlevel% neq 0 exit /b %errorlevel%
cd ..
echo Building backend...
go build -tags desktop,production -ldflags "-H windowsgui -s -w" -o SlackSyncTool.exe main.go app.go
