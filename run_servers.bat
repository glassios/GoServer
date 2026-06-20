@echo off
echo ===================================================
echo Starting Galaxy MMO Distributed Server Stack...
echo (Ensure docker-compose is running if you want Postgres/Redis/NATS,
echo  otherwise servers will run in offline Mock/In-Memory fallback mode)
echo ===================================================

:: Build binaries to ensure latest code is running
echo Building Gateway...
go build -o bin/gateway.exe cmd/gateway/main.go
echo Building WorldNode...
go build -o bin/worldnode.exe cmd/worldnode/main.go
echo Building BotClient...
go build -o bin/botclient.exe cmd/tools/botclient/main.go

echo Starting Gateway Server...
start "Galaxy MMO Gateway" cmd /k "bin\gateway.exe"

echo Starting World Node (System 1)...
start "Galaxy MMO World Node - System 1" cmd /k "bin\worldnode.exe -system-id=1"

echo Starting World Node (System 2)...
start "Galaxy MMO World Node - System 2" cmd /k "bin\worldnode.exe -system-id=2"

echo ===================================================
echo Server components started in separate windows.
echo Web Visualizer is running at: http://localhost:8080/
echo To spawn a bot client for testing, run: bin\botclient.exe
echo (or click "Spawn Bot Client" directly in the Web Visualizer)
echo ===================================================
pause
