# AGENTS.md — 本仓库的 agent 工作约定

## 构建与验证命令

```bash
go vet ./... && go build -o switchproxy .   # 后端校验 + 编译（改 Go 代码后必须跑）
gofmt -l .                                  # 应无输出
cd web && npm run build                     # 前端构建（产物经 go:embed 打进二进制）
./build.sh                                  # 一键：前端 + Linux + Windows 三平台
```

## 端口约定（本机）

- `7899` 被 systemd 占用，本地测试代理端口请用 `127.0.0.1:17898`
- 管理端/面板默认 `127.0.0.1:9090`
- Vite 开发服务器 `5173`（已配置 /api、/ws 反代到 9090）
- 临时测试统一放 `/tmp/opencode`，配置、日志、SQLite 均不要落在仓库目录

## 进程清理：禁用裸 pkill -f（曾在本会话中超时卡死）

agent 的命令由 `zsh -c '<整条命令>'` 包装执行，包装进程的 cmdline 含 pattern 本身，
`pkill -f` 会连宿主 shell 一起杀掉，导致命令无法返回、工具等到超时。

安全做法（按优先级）：

```bash
kill <pid>                          # 1. 启动时记录 PID，按精确 PID 杀
pgrep -af "[n]pm run dev"           # 2. 先看会匹配谁（pgrep -af）
pkill -f "[n]pm run dev"            # 3. 字符类技巧：正则匹配目标，字面量不自匹配
lsof -ti tcp:<port> | xargs -r kill # 4. 按端口杀
timeout 5 pkill -f "[x]xx" || true  # 5. 兜底：加 timeout 且容忍退出码 1
```

后台服务建议 `setsid xxx & echo $! > /tmp/xxx.pid`，关闭时 `kill -- -$(cat /tmp/xxx.pid)` 杀整个进程组。

## 代码风格

- 后端保持 `package main` 平铺多文件（stats/admin/history），不引入框架
- 用户未明确要求时不 commit；不改动已提交的二进制产物
