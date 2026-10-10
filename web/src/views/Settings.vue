<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'

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
  rules: string[]
  upstream: Upstream
  configPath: string
  version: string
}
interface RuleRow {
  type: string
  value: string
  action: string
}

const RULE_TYPES = ['DOMAIN', 'DOMAIN-SUFFIX', 'DOMAIN-KEYWORD', 'IP-CIDR', 'PORT']
const ACTIONS = ['DIRECT', 'PROXY', 'REJECT']

const form = ref<Settings | null>(null)
const saving = ref(false)
const msg = ref<{ ok: boolean; text: string } | null>(null)

const rules = ref<RuleRow[]>([])

const importText = ref('')
const importing = ref(false)
const importMsg = ref<{ ok: boolean; text: string } | null>(null)

const pasteOpen = ref(false)
const pasteText = ref('')
const pasteMsg = ref('')

// 「变更后断开全部连接」开关：保存/导入共用同一个状态，浏览器记忆；仅作为动作选项，不写入配置文件
const closeConns = ref(localStorage.getItem('sp-close-conns') === '1')
watch(closeConns, (v) => localStorage.setItem('sp-close-conns', v ? '1' : '0'))

function loadRules(list: string[]) {
  rules.value = list.map((raw) => {
    const parts = raw.split(',').map((s) => s.trim())
    if (parts.length === 3) {
      return { type: parts[0].toUpperCase(), value: parts[1], action: parts[2].toUpperCase() }
    }
    return { type: 'DOMAIN-SUFFIX', value: raw, action: 'DIRECT' } // 防御：后端已校验，正常不会走到
  })
}

async function load() {
  const res = await fetch('/api/config')
  if (res.ok) {
    form.value = (await res.json()) as Settings
    loadRules(form.value.rules || [])
  }
}

const rowBad = (r: RuleRow) =>
  !r.value.trim() || !RULE_TYPES.includes(r.type) || !ACTIONS.includes(r.action)
const hasBadRows = computed(() => rules.value.some(rowBad))

function rowsToRaw(): string[] {
  return rules.value.map((r) => `${r.type},${r.value.trim()},${r.action}`)
}

async function save() {
  if (!form.value || hasBadRows.value) return
  msg.value = null
  saving.value = true
  try {
    const res = await fetch('/api/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...form.value, rules: rowsToRaw(), closeConns: closeConns.value }),
    })
    if (res.ok) {
      const data = (await res.json()) as { closed: number }
      const target = form.value.monitorListen.trim()
      if (target !== window.location.host) {
        // 管理端口已迁移：旧 origin 即将失效，跳过回读（会失联），直接引导跳转
        msg.value = { ok: true, text: `已生效，面板已迁移，即将跳转 http://${target}` }
        setTimeout(() => {
          location.href = `http://${target}/`
        }, 1500)
        return
      }
      await load()
      msg.value = {
        ok: true,
        text:
          data.closed > 0
            ? `已保存并生效，断开 ${data.closed} 条现有连接（应用会自动重连走新规则）`
            : `已保存并生效（${rules.value.length} 条规则，端口如有变更已即时切换）`,
      }
    } else {
      msg.value = { ok: false, text: await res.text() }
    }
  } finally {
    saving.value = false
  }
}

function addRule() {
  rules.value.push({ type: 'DOMAIN-SUFFIX', value: '', action: 'DIRECT' })
}

function delRule(i: number) {
  rules.value.splice(i, 1)
}

function moveRule(i: number, d: number) {
  const j = i + d
  if (j < 0 || j >= rules.value.length) return
  const [row] = rules.value.splice(i, 1)
  rules.value.splice(j, 0, row)
}

function appendPaste() {
  const lines = pasteText.value.split('\n').map((s) => s.trim()).filter(Boolean)
  if (!lines.length) return
  let bad = 0
  for (const line of lines) {
    const parts = line.split(',').map((s) => s.trim())
    const t = parts[0].toUpperCase()
    const a = parts.length === 3 ? parts[2].toUpperCase() : ''
    if (parts.length === 3 && parts[1] && RULE_TYPES.includes(t) && ACTIONS.includes(a)) {
      rules.value.push({ type: t, value: parts[1], action: a })
    } else {
      bad++
    }
  }
  pasteMsg.value = bad
    ? `已追加 ${lines.length - bad} 条，跳过 ${bad} 条（仅支持五类规则、动作须为 DIRECT/PROXY/REJECT）`
    : `已追加 ${lines.length} 条`
  pasteText.value = ''
}

const PLACEHOLDER: Record<string, string> = {
  DOMAIN: 'www.example.com',
  'DOMAIN-SUFFIX': 'example.com',
  'DOMAIN-KEYWORD': 'google',
  'IP-CIDR': '192.168.0.0/16',
  PORT: '80',
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
  const tail = closeConns.value ? '，并断开全部现有连接' : ''
  if (!window.confirm(`将用导入的配置整体替换当前配置（含监听端口、上级代理与全部规则）${tail}，确定？`)) return
  importing.value = true
  importMsg.value = null
  try {
    const res = await fetch(`/api/config/import${closeConns.value ? '?closeConns=1' : ''}`, {
      method: 'POST',
      body: importText.value,
    })
    if (res.ok) {
      const data = (await res.json()) as { closed: number; monitorListen?: string }
      if (data.monitorListen && data.monitorListen !== window.location.host) {
        // 导入换了管理端口：旧 origin 即将失效，跳过回读直接引导跳转
        importMsg.value = {
          ok: true,
          text: `导入成功，面板已迁移到 http://${data.monitorListen}，即将跳转`,
        }
        setTimeout(() => {
          location.href = `http://${data.monitorListen}/`
        }, 1500)
        return
      }
      importMsg.value = {
        ok: true,
        text: data.closed > 0 ? `导入成功，断开 ${data.closed} 条现有连接` : '导入成功，已整体生效',
      }
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
  </div>

  <div class="panel">
    <h3 class="panel-title">分流规则（从上到下匹配，命中即停）</h3>
    <table class="rules-table">
      <thead>
        <tr>
          <th style="width: 36px">#</th>
          <th style="width: 150px">类型</th>
          <th>值</th>
          <th style="width: 120px">动作</th>
          <th style="width: 84px">顺序</th>
          <th style="width: 44px"></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(r, i) in rules" :key="i" :class="{ bad: rowBad(r) }">
          <td>{{ i + 1 }}</td>
          <td>
            <select v-model="r.type">
              <option v-for="t in RULE_TYPES" :key="t" :value="t">{{ t }}</option>
            </select>
          </td>
          <td><input v-model="r.value" class="mono" :placeholder="PLACEHOLDER[r.type] || ''" /></td>
          <td>
            <select v-model="r.action">
              <option v-for="a in ACTIONS" :key="a" :value="a">{{ a }}</option>
            </select>
          </td>
          <td class="ops">
            <button class="mini" :disabled="i === 0" @click="moveRule(i, -1)">↑</button>
            <button class="mini" :disabled="i === rules.length - 1" @click="moveRule(i, 1)">↓</button>
          </td>
          <td><button class="mini del" @click="delRule(i)">✕</button></td>
        </tr>
        <tr v-if="!rules.length">
          <td colspan="6" class="rules-empty">暂无规则，所有流量走默认动作（{{ form?.default }}）</td>
        </tr>
      </tbody>
    </table>
    <div class="rules-toolbar">
      <button class="mini" @click="addRule">＋ 添加规则</button>
      <button class="mini" @click="pasteOpen = !pasteOpen">批量粘贴</button>
      <span class="hint" v-if="pasteMsg">{{ pasteMsg }}</span>
    </div>
    <div v-if="pasteOpen" class="paste-box">
      <textarea
        v-model="pasteText"
        class="mono"
        placeholder="每行一条 Clash/Surge 格式规则，追加到列表末尾：&#10;DOMAIN-SUFFIX,example.com,DIRECT&#10;IP-CIDR,10.0.0.0/8,DIRECT"
      ></textarea>
      <div class="form-actions">
        <span class="hint">仅支持 DOMAIN / DOMAIN-SUFFIX / DOMAIN-KEYWORD / IP-CIDR / PORT 五类，其余行会被跳过</span>
        <button class="mini" :disabled="!pasteText.trim()" @click="appendPaste">追加到列表</button>
      </div>
    </div>
  </div>

  <div class="panel" v-if="form">
    <div class="form-actions" style="margin-top: 0">
      <span class="hint">
        配置文件：{{ form.configPath }}（自动同步，无需手改；monitor.disable 等冷门项仍在文件中维护）
      </span>
      <div class="save-controls">
        <label class="check" title="保存/导入成功后断开全部已建立的连接，让新规则立即覆盖存量流量；应用会自动重连">
          <input type="checkbox" v-model="closeConns" />
          变更后断开现有连接
        </label>
        <button class="primary" :disabled="saving || hasBadRows" @click="save">
          {{ saving ? '保存中…' : '保存并生效' }}
        </button>
      </div>
    </div>
    <div v-if="closeConns" class="hint" style="margin-top: 8px">
      已开启：每次保存/导入成功后，将断开全部现有连接（关闭此开关则仅新连接按新规则走）
    </div>
    <div v-if="hasBadRows" class="msg err">存在标红的无效规则行（值为空或类型/动作不合法），请修正后再保存</div>
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
      <label class="check" title="与保存按钮共用同一开关：导入成功后断开全部已建立的连接">
        <input type="checkbox" v-model="closeConns" />
        导入后断开现有连接
      </label>
      <button class="primary danger" :disabled="importing || !importText.trim()" @click="doImport">
        {{ importing ? '导入中…' : '导入并替换' }}
      </button>
    </div>
    <div v-if="importMsg" class="msg" :class="importMsg.ok ? 'ok' : 'err'">{{ importMsg.text }}</div>
  </div>
</template>
