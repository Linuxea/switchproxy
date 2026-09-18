# switchproxy

轻量级本地分流代理中间件。部署在应用与下一级代理之间，按规则决定每个连接是**直连出网**还是**转发给上级代理**。

适用场景：使用闭源/不可配置的代理客户端（只提供本地端口），但希望自己控制哪些流量走它、哪些流量直连。

```
应用（浏览器/系统代理）
        │
        ▼
 switchproxy (127.0.0.1:7899, HTTP+SOCKS5 混合入站)
        │  规则引擎（从上到下匹配，命中即停）
        ├── DIRECT ──▶ 直连出网（本机真实出口）
        └── PROXY ──▶ 转发上级代理（HTTP CONNECT / SOCKS5）
```

## 特性

- 单文件 Go 实现，零运行时依赖，交叉编译即用
- 混合入站：同一端口同时支持 HTTP 代理与 SOCKS5
- Clash/Surge 风格规则行（行业标准格式），支持 `DOMAIN` / `DOMAIN-SUFFIX` / `DOMAIN-KEYWORD` / `IP-CIDR` / `PORT`
- 上级代理支持 HTTP 与 SOCKS5，可带 Basic/用户名密码认证
- 配置热重载：保存后 2 秒内生效，不断连接
- 纯隧道转发，不解密不篡改流量：无 MITM、无证书安装、无隐私问题
- 连接级日志（方法 / 目标 / 命中规则 / 动作）

## 快速开始

```bash
# 构建（本地平台）
go build -o switchproxy .

# Windows
GOOS=windows GOARCH=amd64 go build -o switchproxy.exe .

# 运行
cp config.example.yaml config.yaml   # 按需修改规则
./switchproxy -c config.yaml
```

然后把系统/浏览器代理指向 `127.0.0.1:7899`，并关闭下一级代理客户端自己的"系统代理"开关（保持其本地端口监听即可）。

## 配置说明

```yaml
listen: 127.0.0.1:7899        # 混合入站地址
upstream:
  type: http                  # http | socks5
  addr: 127.0.0.1:7890        # 下一级代理
  # username: xxx             # 可选认证
  # password: xxx
default: PROXY                # 未命中规则时: PROXY | DIRECT | REJECT
rules:                        # 从上到下匹配，命中即停
  - DOMAIN-SUFFIX,example.com,DIRECT
  - IP-CIDR,192.168.0.0/16,DIRECT
```

## 说明与限制

- 透明度为**系统代理级**：遵守系统代理的应用全部生效；无视系统代理的进程不会被接管（如需全局接管需 TUN 方案）
- `IP-CIDR` 规则仅对 IP 字面量目标生效，不做主动 DNS 解析
- 纯明文 HTTP 代理转发时会改写请求行为 origin-form（协议要求），HTTPS CONNECT 为纯隧道零改动

## License

MIT
