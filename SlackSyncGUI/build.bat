@echo off
set "PATH=%PATH%;C:\Program Files\Go\bin"
"C:\Users\xiabao\go\bin\wails.exe" build -clean -o SlackSyncTool.exe -v 2 > build_verbose.log 2>&1
