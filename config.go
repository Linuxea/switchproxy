package main

// 配置的写回与应用：设置页（PUT /api/config）与整份导入（POST /api/config/import）
// 共用同一条「校验 -> 落盘 -> 激活 -> 失败回滚」通路。
// 写回用 yaml.Node 做点状更新，保留原文件的注释与规则，只动被编辑的字段。

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var applyMu sync.Mutex // 串行化「写盘 + 激活」，避免与 2 秒热重载轮询竞态

// ---------------- 设置页的数据模型 ----------------

type settingsPayload struct {
	Listen        string `json:"listen"`
	MonitorListen string `json:"monitorListen"`
	Default       string `json:"default"`
	Upstream      struct {
		Type     string `json:"type"`
		Addr     string `json:"addr"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"upstream"`
}

func (p *settingsPayload) validate() error {
	if _, _, err := net.SplitHostPort(p.Listen); err != nil {
		return fmt.Errorf("入站监听地址格式错误（应为 host:port）: %v", err)
	}
	if _, _, err := net.SplitHostPort(p.MonitorListen); err != nil {
		return fmt.Errorf("管理面板监听地址格式错误（应为 host:port）: %v", err)
	}
	p.Default = strings.ToUpper(p.Default)
	switch p.Default {
	case "DIRECT", "PROXY", "REJECT":
	default:
		return fmt.Errorf("默认动作必须是 DIRECT / PROXY / REJECT，got %q", p.Default)
	}
	if p.Upstream.Addr != "" {
		if _, _, err := net.SplitHostPort(p.Upstream.Addr); err != nil {
			return fmt.Errorf("上级代理地址格式错误（应为 host:port）: %v", err)
		}
		p.Upstream.Type = strings.ToLower(p.Upstream.Type)
		if p.Upstream.Type == "" {
			p.Upstream.Type = "http"
		}
		switch p.Upstream.Type {
		case "http", "socks5":
		default:
			return fmt.Errorf("上级代理类型必须是 http / socks5，got %q", p.Upstream.Type)
		}
	}
	return nil
}

// ---------------- yaml.Node 点状更新（保留注释） ----------------

func nodeFind(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func nodeSetStr(m *yaml.Node, key, val string) {
	if v := nodeFind(m, key); v != nil {
		v.SetString(val)
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: val},
	)
}

func nodeDel(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	out := m.Content[:0]
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			out = append(out, m.Content[i], m.Content[i+1])
		}
	}
	m.Content = out
}

// nodeMap 取名为 key 的子 mapping，不存在（或不是 mapping）则重建。
func nodeMap(m *yaml.Node, key string) *yaml.Node {
	if v := nodeFind(m, key); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	nodeDel(m, key)
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		v,
	)
	return v
}

// buildSettingsConfig 在原配置字节上应用设置页的改动，返回新的完整配置字节。
func buildSettingsConfig(orig []byte, p *settingsPayload) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(orig, &doc); err != nil {
		return nil, fmt.Errorf("当前配置文件解析失败: %v", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("配置文件根结构不是映射，无法点状更新")
	}
	root := doc.Content[0]
	nodeSetStr(root, "listen", p.Listen)
	nodeSetStr(root, "default", p.Default)
	if p.Upstream.Addr == "" {
		nodeDel(root, "upstream") // 清空地址 = 删除整段上级配置
	} else {
		up := nodeMap(root, "upstream")
		nodeSetStr(up, "type", p.Upstream.Type)
		nodeSetStr(up, "addr", p.Upstream.Addr)
		if p.Upstream.Username == "" {
			nodeDel(up, "username")
		} else {
			nodeSetStr(up, "username", p.Upstream.Username)
		}
		if p.Upstream.Password == "" {
			nodeDel(up, "password")
		} else {
			nodeSetStr(up, "password", p.Upstream.Password)
		}
	}
	nodeSetStr(nodeMap(root, "monitor"), "listen", p.MonitorListen)
	return yaml.Marshal(&doc)
}

// ---------------- 应用与回滚 ----------------

// activateConfig 切换内存配置并同步监听端口（swap* 对相同地址是 no-op）。
// 端口绑定失败时返回错误：此时规则已生效（current 已换），监听保留在旧地址。
func activateConfig(cfg *Config) error {
	current.Store(cfg)
	var errs []string
	if err := swapProxyListener(cfg.Listen); err != nil {
		errs = append(errs, fmt.Sprintf("入站端口 %s 监听失败: %v", cfg.Listen, err))
	}
	if err := swapAdminListener(cfg.Monitor.Listen); err != nil {
		errs = append(errs, fmt.Sprintf("管理端口 %s 监听失败: %v", cfg.Monitor.Listen, err))
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// applyConfigBytes 校验并整体应用一份新的配置字节：落盘 -> 激活；
// 激活失败（新端口被占用等）则回滚文件与内存，旧端口继续服务。
func applyConfigBytes(newBytes []byte) error {
	applyMu.Lock()
	defer applyMu.Unlock()

	if _, err := parseConfigBytes(newBytes); err != nil {
		return fmt.Errorf("配置校验失败: %v", err)
	}
	oldBytes, _ := os.ReadFile(configPath)
	if err := os.WriteFile(configPath, newBytes, 0644); err != nil {
		return fmt.Errorf("写入配置失败: %v", err)
	}
	cfg, err := loadConfig(configPath)
	if err != nil {
		revertConfig(oldBytes)
		return fmt.Errorf("配置校验失败: %v", err)
	}
	if err := activateConfig(cfg); err != nil {
		log.Println("config apply failed, reverting:", err)
		revertConfig(oldBytes)
		return err
	}
	log.Printf("config applied: %d rules, listen=%s, upstream=%s(%s), default=%s",
		len(cfg.rules), cfg.Listen, cfg.Upstream.Addr, cfg.Upstream.Type, cfg.Default)
	return nil
}

// revertConfig 把文件与内存恢复到旧字节（调用方持有 applyMu）。
func revertConfig(oldBytes []byte) {
	if oldBytes == nil {
		return
	}
	if err := os.WriteFile(configPath, oldBytes, 0644); err != nil {
		log.Println("config revert write failed:", err)
	}
	if oldCfg, err := parseConfigBytes(oldBytes); err == nil {
		current.Store(oldCfg)
		if err := swapProxyListener(oldCfg.Listen); err != nil {
			log.Println("config revert proxy listen:", err)
		}
		if err := swapAdminListener(oldCfg.Monitor.Listen); err != nil {
			log.Println("config revert admin listen:", err)
		}
	}
}

// reloadNow 重新读取配置文件并激活（热重载轮询与手动保存共用）。
func reloadNow() error {
	applyMu.Lock()
	defer applyMu.Unlock()
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if err := activateConfig(cfg); err != nil {
		return fmt.Errorf("规则已生效，但部分端口未切换: %v", err)
	}
	return nil
}
