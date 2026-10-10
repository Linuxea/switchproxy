#!/bin/sh
# 一键构建：前端 -> 内嵌 -> Go 二进制（Linux + Windows portable 包）
set -e
cd "$(dirname "$0")"

VER="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X main.version=$VER"

echo "==> building web..."
cd web
npm install
npm run build
cd ..

echo "==> building switchproxy (linux)..."
go build -trimpath -ldflags "$LDFLAGS" -o switchproxy .

echo "==> building switchproxy (windows/amd64, windowsgui 无控制台 + 托盘)..."
mkdir -p dist/switchproxy
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS -H windowsgui" -o dist/switchproxy/switchproxy.exe .
cp config.example.yaml dist/switchproxy/
cat > dist/switchproxy/使用说明.txt <<EOF
switchproxy $VER (Windows)

1. 双击 switchproxy.exe 运行（无控制台黑框，托盘出现图标）
   - 首次运行自动在同目录生成 config.yaml，日志写 switchproxy.log
2. 托盘图标右键 -> 「打开面板」打开监控/设置页面
3. 浏览器或系统代理指向配置中 listen 端口（默认 127.0.0.1:7899）
4. 面板「设置」页可改监听端口、配置上级代理、导入整份配置
EOF

(cd dist && zip -r -q "switchproxy-windows-amd64-$VER.zip" switchproxy)

echo "==> done."
echo "  linux:   ./switchproxy"
echo "  windows: dist/switchproxy-windows-amd64-$VER.zip"
