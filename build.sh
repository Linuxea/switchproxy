#!/bin/sh
# 一键构建：前端 -> 内嵌 -> Go 二进制
set -e
cd "$(dirname "$0")"

echo "==> building web..."
cd web
npm install
npm run build
cd ..

echo "==> building switchproxy (linux)..."
go build -o switchproxy .

echo "==> building switchproxy (windows/amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o switchproxy-windows-amd64.exe .

echo "==> done."
./switchproxy -c config.example.yaml 2>&1 | head -2 || true
echo "运行 ./switchproxy -c config.yaml 后访问 http://127.0.0.1:9090"
