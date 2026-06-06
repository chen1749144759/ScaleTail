<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Copy, LayoutDashboard, LogOut, PlugZap, PowerOff } from "lucide-vue-next";
import StatusPill from "../components/StatusPill.vue";
import type { Prefs, Status } from "../../shared/types";

const emit = defineEmits<{
  "open-dashboard": [];
}>();

const status = ref<Status | null>(null);
const prefs = ref<Prefs | null>(null);
const serverIP = ref("");
const serverPort = ref("80");
const useHTTPS = ref(false);
const hostname = ref("");
const authKey = ref("");
const acceptRoutes = ref(true);
const loading = ref(false);
const message = ref("");
const error = ref("");

const backendState = computed(() => status.value?.BackendState || "");
const activeState = computed(() => ["Running", "Starting", "NeedsMachineAuth"].includes(backendState.value));
const canResume = computed(() => backendState.value === "Stopped" && Boolean(status.value?.HaveNodeKey));
const configLocked = computed(() => activeState.value || canResume.value);
const canLogout = computed(() => activeState.value || canResume.value);
const controlURL = computed(() => {
  const scheme = useHTTPS.value ? "https" : "http";
  const port = serverPort.value.trim() || (useHTTPS.value ? "443" : "80");
  return `${scheme}://${serverIP.value.trim()}:${port}`;
});
const commandLine = computed(() => {
  const args = [
    "tailscale",
    "up",
    `--login-server=${quoteArg(controlURL.value)}`,
    `--accept-routes=${acceptRoutes.value ? "true" : "false"}`,
  ];
  if (hostname.value.trim()) {
    args.push(`--hostname=${quoteArg(hostname.value.trim())}`);
  }
  if (authKey.value.trim()) {
    args.push(`--auth-key=${quoteArg(authKey.value.trim())}`);
  }
  return args.join(" ");
});

onMounted(() => {
  void load();
});

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [nextStatus, nextPrefs] = await Promise.all([
      window.tailscale.getStatus(false),
      window.tailscale.getPrefs(),
    ]);
    status.value = nextStatus;
    prefs.value = nextPrefs;
    parseControlURL(nextPrefs.ControlURL || "");
    hostname.value = nextPrefs.Hostname || "";
    if (typeof nextPrefs.RouteAll === "boolean") {
      acceptRoutes.value = nextPrefs.RouteAll;
    }
    if (!serverPort.value) {
      serverPort.value = useHTTPS.value ? "443" : "80";
    }
    if (configLocked.value) {
      message.value = lockMessage(backendState.value);
    }
  } catch (err) {
    error.value = messageOf(err);
  } finally {
    loading.value = false;
  }
}

async function connect() {
  if (configLocked.value) {
    return;
  }
  loading.value = true;
  message.value = "正在提交连接请求...";
  error.value = "";
  try {
    const res = await window.tailscale.connect({
      serverIP: serverIP.value,
      serverPort: serverPort.value,
      useHTTPS: useHTTPS.value,
      hostname: hostname.value,
      authKey: authKey.value,
      acceptRoutes: acceptRoutes.value,
    });
    message.value = res.message;
    await load();
  } catch (err) {
    error.value = messageOf(err);
    message.value = "";
  } finally {
    loading.value = false;
  }
}

async function disconnectCurrent() {
  loading.value = true;
  error.value = "";
  message.value = "正在临时断开连接...";
  try {
    const res = await window.tailscale.disconnect();
    message.value = res.message;
    await load();
  } catch (err) {
    error.value = messageOf(err);
    message.value = "";
  } finally {
    loading.value = false;
  }
}

async function reconnectCurrent() {
  loading.value = true;
  error.value = "";
  message.value = "正在恢复连接...";
  try {
    const res = await window.tailscale.reconnect();
    message.value = res.message;
    await load();
  } catch (err) {
    error.value = messageOf(err);
    message.value = "";
  } finally {
    loading.value = false;
  }
}

async function logoutCurrent() {
  if (!confirm("确定退出当前网络吗？这会清除当前登录状态和本机节点身份。之后如需重新加入网络，需要重新填写有效预认证密钥，或在 Headscale 服务端手动注册。")) {
    return;
  }
  loading.value = true;
  error.value = "";
  message.value = "正在退出当前网络...";
  try {
    await window.tailscale.logout();
    message.value = "已退出当前网络，现在可以修改服务端配置。";
    await load();
  } catch (err) {
    error.value = messageOf(err);
    message.value = "";
  } finally {
    loading.value = false;
  }
}

async function copyCommand() {
  try {
    await navigator.clipboard.writeText(commandLine.value);
    message.value = "命令已复制。";
    error.value = "";
  } catch {
    error.value = "复制失败，请手动选中命令文本复制。";
  }
}

function parseControlURL(raw: string) {
  if (!raw) {
    return;
  }
  try {
    const parsed = new URL(raw);
    serverIP.value = parsed.hostname || "";
    useHTTPS.value = parsed.protocol === "https:";
    serverPort.value = parsed.port || (useHTTPS.value ? "443" : "80");
  } catch {
    // Keep the current form values if older prefs contain an unexpected URL.
  }
}

function quoteArg(value: string) {
  if (!value) {
    return "\"\"";
  }
  return /[\s"&|<>^]/.test(value) ? `"${value.replace(/"/g, "\\\"")}"` : value;
}

function lockMessage(state: string) {
  if (state === "Starting") {
    return "当前正在恢复连接，服务端配置已临时锁定。";
  }
  if (state === "NeedsMachineAuth") {
    return "当前连接正在等待设备授权，服务端配置已临时锁定。";
  }
  if (state === "Stopped") {
    return "当前已临时断开，登录状态仍保留。可直接恢复连接；如需更换服务端，请先退出当前网络。";
  }
  return "当前已连接，服务端配置已锁定。需要连接其他服务端时，请先退出当前网络。";
}

function messageOf(err: unknown) {
  return err instanceof Error ? err.message : String(err || "未知错误");
}
</script>

<template>
  <section class="connect-layout">
    <div class="connect-panel">
      <div class="section-head">
        <div>
          <h2>服务端连接</h2>
          <p>填写 Headscale/Tailscale 控制服务器地址，连接会通过本地服务完成。</p>
        </div>
        <StatusPill :state="backendState" />
      </div>

      <div v-if="error" class="notice error compact">
        <strong>操作失败</strong>
        <p>{{ error }}</p>
      </div>
      <div v-if="message" class="notice ok compact">
        <strong>提示</strong>
        <p>{{ message }}</p>
      </div>

      <div class="form-grid">
        <label class="field wide">
          <span>服务端 IP 或域名</span>
          <input v-model="serverIP" :disabled="configLocked" type="text" placeholder="192.168.1.10 或 headscale.example.com" />
        </label>
        <label class="field">
          <span>端口</span>
          <input v-model="serverPort" :disabled="configLocked" type="text" inputmode="numeric" placeholder="80" />
        </label>
      </div>

      <label class="field">
        <span>本机设备名称，可选</span>
        <input v-model="hostname" :disabled="configLocked" type="text" placeholder="留空使用系统主机名，例如 office-pc" />
      </label>

      <div class="checks">
        <label>
          <input v-model="useHTTPS" :disabled="configLocked" type="checkbox" />
          使用 HTTPS
        </label>
        <label>
          <input v-model="acceptRoutes" :disabled="configLocked" type="checkbox" />
          接受路由
        </label>
      </div>

      <div class="preview mono">{{ controlURL }}</div>

      <label class="field">
        <span>预认证密钥，可选</span>
        <input v-model="authKey" :disabled="configLocked" type="password" placeholder="tskey-auth-... 或 hskey-auth-..." />
      </label>

      <div class="command-head">
        <strong>等价命令</strong>
        <button class="btn" @click="copyCommand">
          <Copy :size="16" />
          复制
        </button>
      </div>
      <textarea class="command mono" :value="commandLine" readonly />

      <div class="toolbar">
        <button v-if="canResume" class="btn primary" :disabled="loading" @click="reconnectCurrent">
          <PlugZap :size="16" />
          恢复连接
        </button>
        <button v-else-if="!activeState" class="btn primary" :disabled="loading" @click="connect">
          <PlugZap :size="16" />
          连接
        </button>
        <button v-if="activeState" class="btn" :disabled="loading" @click="disconnectCurrent">
          <PowerOff :size="16" />
          断开连接
        </button>
        <button v-if="canLogout" class="btn danger" :disabled="loading" @click="logoutCurrent">
          <LogOut :size="16" />
          退出当前网络
        </button>
        <button class="btn" @click="emit('open-dashboard')">
          <LayoutDashboard :size="16" />
          打开仪表盘
        </button>
      </div>
    </div>
  </section>
</template>
