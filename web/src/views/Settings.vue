<script setup lang="ts">
import { onMounted, ref } from 'vue'

interface Upstream {
  type: string
  addr: string
  username: string
  password: string
}
interface Settings {
  listen: string
  monitorListen: string
  default: string
  upstream: Upstream
  ruleCount: number
  configPath: string
  version: string
}

const form = ref<Settings | null>(null)
const saving = ref(false)
const msg = ref<{ ok: boolean; text: string } | null>(null)

const importText = ref('')
const importing = ref(false)
const importMsg = ref<{ ok: boolean; text: string } | null>(null)

async function load() {
  const res = await fetch('/api/config')
  if (res.ok) form.value = (await res.json()) as Settings
}

async function save() {
  if (!form.value) return
  msg.value = null
  saving.value = true
  try {
    const res = await fetch('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(form.value),
    })
    if (res.ok) {
      const oldMonitor = form.value.monitorListen
      await load()
      if (form.value.monitorListen !== oldMonitor) {
        // 管理端口变了：本页面很快会失联，指引用户跳到新地址
        msg.value = { ok: true, text: `已生效，面板已迁移，即将跳转 http://${form.value.monitorListen}` }
        setTimeout(() => {
          location.href = `http://${form.value.monitorListen}/`
        }, 1500)
      } else {
        msg.value = { ok: true, text: '已保存并生效（端口如有变更已即时切换）' }
      }
    } else {
      msg.value = { ok: false, text: await res.text() }
    }
  } finally {
    saving.value = false
  }
}

function onFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    importText.value = String(reader.result || '')
  }
  reader.readAsText(file)
}

async function doImport() {
  if (!importText.value.trim()) return
  if (!window.confirm('将用导入的配置整体替换当前配置（含监听端口、上级代理与全部规则），确定？')) return
  importing.value = true
  importMsg.value = null
  try {
    const res = await fetch('/api/config/import', { method: 'POST', body: importText.value })
    if (res.ok) {
      importMsg.value = { ok: true, text: '导入成功，已整体生效' }
      importText.value = ''
      await load()
    } else {
      importMsg.value = { ok: false, text: await res.text() }
    }
  } finally {
    importing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="panel" v-if="form">
    <h3 class="panel-title">基本设置</h3>
    <div class="form">
      <div class="field">
        <label>入站监听（HTTP + SOCKS5 混合端口）</label>
        <input v-model="form.listen" class="mono" placeholder="127.0.0.1:7899" />
        <p class="hint">应用/系统代理指向这里；保存后即时切换，存量连接不断</p>
      </div>
      <div class="field">
        <label>管理面板监听</label>
        <input v-model="form.monitorListen" class="mono" placeholder="127.0.0.1:9090" />
        <p class="hint">本页面地址，改动后会自动跳转到新地址</p>
      </div>
      <div class="field">
        <label>未命中规则的默认动作</label>
        <select v-model="form.default">
          <option value="PROXY">PROXY（走上级代理）</option>
          <option value="DIRECT">DIRECT（直连）</option>
          <option value="REJECT">REJECT（拒绝）</option>
        </select>
      </div>
    </div>

    <h3 class="panel-title" style="margin-top: 16px">上级代理</h3>
    <div class="form">
      <div class="field">
        <label>类型</label>
        <select v-model="form.upstream.type">
          <option value="http">HTTP</option>
          <option value="socks5">SOCKS5</option>
        </select>
      </div>
      <div class="field">
        <label>地址</label>
        <input v-model="form.upstream.addr" class="mono" placeholder="127.0.0.1:7890" />
        <p class="hint">留空 = 不使用上级代理（整段配置会被移除）</p>
      </div>
      <div class="field">
        <label>用户名（可选）</label>
        <input v-model="form.upstream.username" autocomplete="off" />
      </div>
      <div class="field">
        <label>密码（可选）</label>
        <input v-model="form.upstream.password" type="password" autocomplete="new-password" />
      </div>
    </div>

    <div class="form-actions">
      <span class="hint">
        配置文件：{{ form.configPath }} · 当前规则 {{ form.ruleCount }} 条（规则请在配置文件中维护，支持热重载）
      </span>
      <button class="primary" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存并生效' }}</button>
    </div>
    <div v-if="msg" class="msg" :class="msg.ok ? 'ok' : 'err'">{{ msg.text }}</div>
  </div>

  <div class="panel">
    <h3 class="panel-title">导入配置</h3>
    <p class="hint" style="margin: 0 0 10px">
      粘贴或选择一份 switchproxy 格式的配置文件，校验通过后<strong>整体替换</strong>当前配置（含端口、上级代理与全部规则）。
    </p>
    <div class="field" style="margin-bottom: 10px">
      <input type="file" accept=".yaml,.yml,.txt" @change="onFile" />
    </div>
    <textarea
      v-model="importText"
      class="mono"
      placeholder="# 粘贴 config.yaml 内容，或上面的文件选择后自动填入&#10;listen: 127.0.0.1:7899&#10;rules:&#10;  - DOMAIN-SUFFIX,example.com,DIRECT"
    ></textarea>
    <div class="form-actions">
      <span></span>
      <button class="primary danger" :disabled="importing || !importText.trim()" @click="doImport">
        {{ importing ? '导入中…' : '导入并替换' }}
      </button>
    </div>
    <div v-if="importMsg" class="msg" :class="importMsg.ok ? 'ok' : 'err'">{{ importMsg.text }}</div>
  </div>
</template>
