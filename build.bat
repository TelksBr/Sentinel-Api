@echo off
REM Script de build para Linux (desenvolvido no Windows)

if not exist build mkdir build

echo Building for Linux x64 (static)...
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -a -installsuffix cgo -ldflags="-s -w" -o build/api-v2_x64 ./cmd/api

echo Building for Linux ARM64 (static)...
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=arm64
go build -a -installsuffix cgo -ldflags="-s -w" -o build/api-v2_arm64 ./cmd/api

echo Building static binary for Linux x64...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -a -installsuffix cgo -ldflags="-s -w" -o build/api-v2_static ./cmd/api

echo All Linux builds completed!
echo.
echo Binaries created:
dir build\api-v2_*
