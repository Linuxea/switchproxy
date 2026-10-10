//go:build windows

package main

// Windows 桌面壳的杂项：日志落盘、默认配置定位与首次生成。
// 与托盘（tray_windows.go）分开，是因为这里全是「启动早于配置加载」的事。

import (
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
)

// setupPlatform 在 flag.Parse 之后、loadConfig 之前调用。
func setupPlatform() {
	dir := exeDir()
	setupWindowsLog(dir)
	defaultConfigNearExe(dir)
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// setupWindowsLog 同时写 stderr 与 exe 旁日志文件。
// windowsgui 编译后没有控制台，stderr 写入无效但无害（此时日志只剩文件一路）；
// 从终端启动调试时两路都活着。超过 5MB 单代轮转。
func setupWindowsLog(dir string) {
	p := filepath.Join(dir, "switchproxy.log")
	if fi, err := os.Stat(p); err == nil && fi.Size() > 5<<20 {
		os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return // 目录只读等场景：退回仅 stderr
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

// defaultConfigNearExe 未显式指定 -c 时，配置定位到 exe 目录（双击运行的 CWD 不可靠）；
// 且文件不存在时自动生成一份默认配置——否则 windowsgui 下 loadConfig 失败会静默退出，
// 用户只看到「双击没反应」。
func defaultConfigNearExe(dir string) {
	if f := flag.Lookup("c"); f != nil && f.Value.String() != f.DefValue {
		return // 用户显式指定了配置路径
	}
	configPath = filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		return
	}
	if err := os.WriteFile(configPath, []byte(windowsDefaultConfig), 0644); err != nil {
		log.Println("create default config:", err)
		return
	}
	log.Printf("created default config: %s", configPath)
}

// windowsDefaultConfig 首次运行生成的默认配置：无上级代理、默认 DIRECT，
// 即「透明直连 + 面板可用」的安全起点，用户在设置页填好上级后再改 default。
const windowsDefaultConfig = `# switchproxy 默认配置（首次运行自动生成，可在面板「设置」页修改或整份导入）
# listen: 本地混合入站端口，同时支持 HTTP 代理和 SOCKS5
listen: 127.0.0.1:7899

# upstream: 下一级代理（type 支持 http / socks5）；暂不需要可保持注释
# upstream:
#   type: http
#   addr: 127.0.0.1:7890
#   # username: xxx
#   # password: xxx

# default: 未命中任何规则时的动作 PROXY / DIRECT / REJECT
default: DIRECT

# 规则从上到下匹配，命中即停（与 Clash/Surge 规则行格式一致）
# 支持: DOMAIN / DOMAIN-SUFFIX / DOMAIN-KEYWORD / IP-CIDR / PORT
rules:
  - IP-CIDR,192.168.0.0/16,DIRECT
  - IP-CIDR,10.0.0.0/8,DIRECT
  - IP-CIDR,127.0.0.0/8,DIRECT

# 实时流量监控面板（可选，缺省即启用）
monitor:
  listen: 127.0.0.1:9090   # 管理端地址，浏览器打开即面板
  db: switchproxy.db       # SQLite 历史库
`
