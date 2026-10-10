//go:build !windows

package main

// 非 Windows 平台的桌面集成空实现：保持 Linux/macOS 上编译与运行行为不变。

func setupPlatform() {}

func runPlatformTray() {}
