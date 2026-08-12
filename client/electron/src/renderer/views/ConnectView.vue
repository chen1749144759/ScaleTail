<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { LayoutDashboard, LoaderCircle, LogOut, PlugZap, PowerOff } from "lucide-vue-next";
import StatusPill from "../components/StatusPill.vue";
import type { PasswordChangeProgress, Prefs, Status } from "../../shared/types";

const emit = defineEmits<{
  "open-dashboard": [];
}>();

const status = ref<Status | null>(null);
const prefs = ref<Prefs | null>(null);
const serverIP = ref("");
const serverPort = ref("443");
const useHTTPS = ref(true);
const username = ref("");
const password = ref("");
const acceptRoutes = ref(true);
const acceptDNS = ref(true);
const loading = ref(false);
const message = ref("");
const error = ref("");
const logoutConfirmOpen = ref(false);
const passwordChangeOpen = ref(false);
const pendingCurrentPassword = ref("");
const newPassword = ref("");
const confirmNewPassword = ref("");
const passwordChangeRequiresRegistrationSession = ref(true);
const passwordChangeLoading = ref(false);
const passwordChangeError = ref("");
const passwordChangeProgress = ref<PasswordChangeProgress>("preparing");
let passwordChangeAttempt = 0;
let offPasswordChangeProgress: (() => void) | undefined;

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
const connectionPreview = computed(() => {
  const lines = [
    `控制服务器  ${controlURL.value}`,
    `登录账号    ${username.value.trim() || "（未填写）"}`,
    `接受路由    ${acceptRoutes.value ? "是" : "否"}`,
    `采用 DNS    ${acceptDNS.value ? "是" : "否"}`,
  ];
  return lines.join("\n");
});

onMounted(() => {
  offPasswordChangeProgress = window.scaletail.onPasswordChangeProgress((stage) => {
    passwordChangeProgress.value = stage;
  });
  void load();
});

onUnmounted(() => {
  offPasswordChangeProgress?.();
  if (passwordChangeLoading.value && passwordChangeProgress.value === "preparing") {
    void window.scaletail.cancelPasswordChange();
  }
});

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [nextStatus, nextPrefs] = await Promise.all([
      window.scaletail.getStatus(false),
      window.scaletail.getPrefs(),
    ]);
    status.value = nextStatus;
    prefs.value = nextPrefs;
    parseControlURL(nextPrefs.ControlURL || "");
    if (typeof nextPrefs.RouteAll === "boolean") {
      acceptRoutes.value = nextPrefs.RouteAll;
    }
    if (typeof nextPrefs.CorpDNS === "boolean") {
      acceptDNS.value = nextPrefs.CorpDNS;
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
    const res = await window.scaletail.connect({
      serverIP: serverIP.value,
      serverPort: serverPort.value,
      useHTTPS: useHTTPS.value,
      hostname: "",
      username: username.value,
      password: password.value,
      acceptRoutes: acceptRoutes.value,
      acceptDNS: acceptDNS.value,
    });
    if (res.passwordChangeRequired) {
      pendingCurrentPassword.value = password.value;
      newPassword.value = "";
      confirmNewPassword.value = "";
      passwordChangeRequiresRegistrationSession.value = res.passwordChangeRequiresRegistrationSession !== false;
      passwordChangeError.value = "";
      passwordChangeProgress.value = "preparing";
      passwordChangeOpen.value = true;
      message.value = "";
      return;
    }
    message.value = res.message;
    await load();
  } catch (err) {
    error.value = messageOf(err);
    message.value = "";
  } finally {
    password.value = "";
    loading.value = false;
  }
}

async function submitPasswordChange() {
  passwordChangeError.value = "";
  const newPasswordBytes = new TextEncoder().encode(newPassword.value).length;
  if (newPasswordBytes < 12 || newPasswordBytes > 72) {
    passwordChangeError.value = "新密码需要 12 到 72 个字节。";
    return;
  }
  if (newPassword.value !== confirmNewPassword.value) {
    passwordChangeError.value = "两次输入的新密码不一致。";
    return;
  }
  if (newPassword.value === pendingCurrentPassword.value) {
    passwordChangeError.value = "新密码不能与初始密码相同。";
    return;
  }

  const attempt = ++passwordChangeAttempt;
  passwordChangeLoading.value = true;
  passwordChangeProgress.value = "preparing";
  error.value = "";
  message.value = "";
  try {
    const res = await window.scaletail.changeExpiredPassword({
      serverIP: serverIP.value,
      serverPort: serverPort.value,
      useHTTPS: useHTTPS.value,
      hostname: "",
      username: username.value,
      password: pendingCurrentPassword.value,
      newPassword: newPassword.value,
      requireRegistrationSession: passwordChangeRequiresRegistrationSession.value,
      acceptRoutes: acceptRoutes.value,
      acceptDNS: acceptDNS.value,
    });
    if (attempt !== passwordChangeAttempt) return;
    password.value = "";
    pendingCurrentPassword.value = "";
    newPassword.value = "";
    confirmNewPassword.value = "";
    passwordChangeOpen.value = false;
    message.value = res.message;
    await load();
  } catch (err) {
    if (attempt !== passwordChangeAttempt) return;
    const detail = messageOf(err);
    if (detail.startsWith("新密码已设置")) {
      password.value = "";
      pendingCurrentPassword.value = "";
      newPassword.value = "";
      confirmNewPassword.value = "";
      passwordChangeOpen.value = false;
    }
    if (detail !== "操作已取消") {
      passwordChangeError.value = detail;
    }
  } finally {
    if (attempt === passwordChangeAttempt) {
      passwordChangeLoading.value = false;
    }
  }
}

function cancelPasswordChange() {
  passwordChangeAttempt += 1;
  if (passwordChangeLoading.value && passwordChangeProgress.value === "preparing") {
    void window.scaletail.cancelPasswordChange();
  }
  passwordChangeLoading.value = false;
  passwordChangeOpen.value = false;
  passwordChangeError.value = "";
  pendingCurrentPassword.value = "";
  newPassword.value = "";
  confirmNewPassword.value = "";
}

function passwordChangeStatus() {
  if (passwordChangeProgress.value === "updating") return "正在设置新密码...";
  if (passwordChangeProgress.value === "connecting") return "密码已设置，正在连接...";
  return "正在准备连接...";
}

async function disconnectCurrent() {
  loading.value = true;
  error.value = "";
  message.value = "正在临时断开连接...";
  try {
    const res = await window.scaletail.disconnect();
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
    const res = await window.scaletail.reconnect();
    message.value = res.message;
    await load();
  } catch (err) {
    error.value = messageOf(err);
    message.value = "";
  } finally {
    loading.value = false;
  }
}

function requestLogout() {
  logoutConfirmOpen.value = true;
}

async function logoutCurrent() {
  logoutConfirmOpen.value = false;
  loading.value = true;
  error.value = "";
  message.value = "正在退出当前网络...";
  try {
    await window.scaletail.logout();
    username.value = "";
    password.value = "";
    message.value = "已退出当前网络，现在可以修改服务端配置。";
    await load();
  } catch (err) {
    error.value = messageOf(err);
    message.value = "";
  } finally {
    loading.value = false;
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

      <div class="checks">
        <label>
          <input v-model="useHTTPS" :disabled="configLocked" type="checkbox" />
          使用 HTTPS
        </label>
        <label>
          <input v-model="acceptRoutes" :disabled="configLocked" type="checkbox" />
          接受路由
        </label>
        <label>
          <input v-model="acceptDNS" :disabled="configLocked" type="checkbox" />
          采用服务端 DNS
        </label>
      </div>

      <div class="preview mono">{{ controlURL }}</div>

      <div class="form-grid account-grid">
        <label class="field">
          <span>账号</span>
          <input v-model="username" :disabled="configLocked" type="text" autocomplete="off" placeholder="请输入账号" />
        </label>
        <label class="field">
          <span>密码</span>
          <input v-model="password" :disabled="configLocked" type="password" autocomplete="off" placeholder="请输入密码" />
        </label>
      </div>

      <div class="command-head">
        <strong>连接参数预览</strong>
      </div>
      <textarea class="command mono" :value="connectionPreview" readonly />

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
        <button v-if="canLogout" class="btn danger" :disabled="loading" @click="requestLogout">
          <LogOut :size="16" />
          退出当前网络
        </button>
        <button class="btn" @click="emit('open-dashboard')">
          <LayoutDashboard :size="16" />
          打开仪表盘
        </button>
      </div>
    </div>

    <div v-if="logoutConfirmOpen" class="modal-backdrop" @click.self="logoutConfirmOpen = false">
      <section class="confirm-modal" role="dialog" aria-modal="true" aria-labelledby="logout-title">
        <div>
          <span class="modal-kicker">退出当前网络</span>
          <h3 id="logout-title">清除当前登录状态？</h3>
          <p>
            这会删除本机节点身份并解锁服务端配置。之后如需重新加入网络，需要重新输入账号密码完成认证。
          </p>
        </div>
        <div class="modal-actions">
          <button class="btn" :disabled="loading" @click="logoutConfirmOpen = false">取消</button>
          <button class="btn danger solid" :disabled="loading" @click="logoutCurrent">
            <LogOut :size="16" />
            确认退出
          </button>
        </div>
      </section>
    </div>

    <div v-if="passwordChangeOpen" class="modal-backdrop" @click.self="cancelPasswordChange">
      <section class="confirm-modal password-change-modal" role="dialog" aria-modal="true" aria-labelledby="password-change-title">
        <div>
          <span class="modal-kicker password-kicker">首次登录保护</span>
          <h3 id="password-change-title">设置你的新密码</h3>
          <p>首次登录需要设置新密码，完成后会自动连接。</p>
        </div>
        <label class="field">
          <span>新密码</span>
          <input v-model="newPassword" :disabled="passwordChangeLoading" type="password" autocomplete="new-password" placeholder="12 到 72 字节" />
        </label>
        <label class="field">
          <span>确认新密码</span>
          <input v-model="confirmNewPassword" :disabled="passwordChangeLoading" type="password" autocomplete="new-password" placeholder="再次输入新密码" @keyup.enter="submitPasswordChange" />
        </label>
        <div v-if="passwordChangeError" class="modal-error" role="alert">{{ passwordChangeError }}</div>
        <div v-if="passwordChangeLoading" class="modal-progress" role="status">
          <LoaderCircle :size="16" class="spin" />
          {{ passwordChangeStatus() }}
        </div>
        <div class="modal-actions">
          <button class="btn" @click="cancelPasswordChange">取消</button>
          <button class="btn primary" :disabled="passwordChangeLoading" @click="submitPasswordChange">
            <LoaderCircle v-if="passwordChangeLoading" :size="16" class="spin" />
            设置并连接
          </button>
        </div>
      </section>
    </div>
  </section>
</template>
