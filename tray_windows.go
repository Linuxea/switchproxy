//go:build windows

package main

// 极简托盘：打开面板 / 退出。
// windowsgui 程序没有控制台窗口，托盘退出是唯一的优雅关闭入口，所以必须有它。

import (
	_ "embed"
	"log"
	"os/exec"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var trayIcon []byte

// runPlatformTray 启动托盘（不阻塞调用方）。
func runPlatformTray() {
	go systray.Run(onTrayReady, nil)
}

func onTrayReady() {
	systray.SetIcon(trayIcon)
	systray.SetTooltip("switchproxy " + version)
	title := systray.AddMenuItem("switchproxy "+version, "")
	title.Disable()
	systray.AddSeparator()
	open := systray.AddMenuItem("打开面板", "在浏览器中打开监控面板")
	systray.AddSeparator()
	quit := systray.AddMenuItem("退出", "关闭 switchproxy")
	go func() {
		for {
			select {
			case <-open.ClickedCh:
				cfg := current.Load().(*Config)
				if !cfg.Monitor.Disable {
					openBrowser("http://" + cfg.Monitor.Listen)
				}
			case <-quit.ClickedCh:
				systray.Quit() // 先让托盘图标消失，再走统一退出
				shutdown()
			}
		}
	}()
}

func openBrowser(u string) {
	// rundll32 是 GUI 程序，不会像 cmd /c start 那样闪一下控制台
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start(); err != nil {
		log.Println("open browser:", err)
	}
}
