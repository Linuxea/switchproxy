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
- **内置实时流量监控面板**：总览速率 / 连接明细 / 域名规则排行 / SQLite 历史回看
- **面板设置页**：改监听端口 / 配置上级代理 / 导入整份配置，即存即生效（端口热切换，存量连接不断；失败自动回滚）
- **Windows 桌面体验**：无控制台黑框、托盘常驻（打开面板 / 退出）、首次运行自动生成配置、日志落盘

## 快速开始

```bash
# 构建（本地平台，前端已内嵌）
go build -o switchproxy .

# 或一键构建：前端 + Linux + Windows
./build.sh

# 前端开发模式（热重载联调，自动代理 API/WS 到 9090）
cd web && npm install && npm run dev   # http://127.0.0.1:5173

# 运行
cp config.example.yaml config.yaml   # 按需修改规则
./switchproxy -c config.yaml
```

然后打开监控面板 `http://127.0.0.1:9090`，再把系统/浏览器代理指向 `127.0.0.1:7899`，并关闭下一级代理客户端自己的"系统代理"开关（保持其本地端口监听即可）。

### Windows

```sh
./build.sh    # 产出 dist/switchproxy-windows-amd64-<ver>.zip（portable 绿色包）
```

解压后双击 `switchproxy.exe`：

- 无控制台黑框（windowsgui 子系统），托盘区出现图标，右键菜单：**打开面板 / 退出**
- 首次运行在 exe 同目录自动生成 `config.yaml`（默认无上级代理、DIRECT，安全起点），日志写同目录 `switchproxy.log`
- 一切端口 / 上级代理 / 规则调整都可以在面板「设置」页完成，也可以整份导入已有配置

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
monitor:                      # 实时监控面板（缺省即启用）
  listen: 127.0.0.1:9090      # 管理端地址（面板 + REST API + WebSocket）
  db: switchproxy.db          # SQLite 历史库；流量点保留 30 天，域名统计 90 天
  # disable: true             # 设为 true 关闭监控
```

## 监控面板

- **总览**：实时上/下行速率曲线、累计流量、活跃/累计连接数、运行时间
- **连接**：活跃连接实时表（协议/目标/命中规则/动作/双向速率与流量/时长，可远程强制关闭）+ 最近关闭记录
- **排行**：按域名、按规则的流量 TOP N（WebSocket 每秒推送）
- **历史**：1h / 24h / 7d / 30d 流量曲线与域名排行（SQLite 分钟级落盘）
- **设置**：入站/管理监听端口、默认动作、上级代理表单；导入整份配置（Clash 风格规则行的现有格式）
  - 保存与导入都是**先校验后落盘**：改端口即时热切换（存量连接不断），新端口被占用则整体回滚并报错
  - 设置写回走 yaml.Node 点状更新，**保留原文件注释与规则**；导入则整份替换

管理 API（默认仅本机监听）：`GET /api/overview`、`GET /api/connections`、`POST /api/connections/{id}/close`、`GET /api/tops`、`GET /api/history?range=1h|24h|7d|30d`、`GET /ws`（WebSocket 每秒推送全量快照）、`GET|PUT /api/config`（读/改设置）、`POST /api/config/import`（body 为整份 YAML，校验通过后替换）。

## 说明与限制

- 透明度为**系统代理级**：遵守系统代理的应用全部生效；无视系统代理的进程不会被接管（如需全局接管需 TUN 方案）
- `IP-CIDR` 规则仅对 IP 字面量目标生效，不做主动 DNS 解析
- 纯明文 HTTP 代理转发时会改写请求行为 origin-form（协议要求），HTTPS CONNECT 为纯隧道零改动

## License

MIT
