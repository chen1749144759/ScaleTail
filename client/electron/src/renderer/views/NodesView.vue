<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { RefreshCw, Router, Save } from "lucide-vue-next";
import type { NetcheckReport, PeerStatus, Prefs, Status } from "../../shared/types";

const status = ref<Status | null>(null);
const prefs = ref<Prefs | null>(null);
const netcheck = ref<NetcheckReport | null>(null);
const loading = ref(false);
const netcheckLoading = ref(false);
const routeLoading = ref(false);
const exitNodeLoading = ref(false);
const error = ref("");
const message = ref("");
const exitNodeID = ref("");
const exitNodeTouched = ref(false);
const advertiseRoutes = ref("");
const routesTouched = ref(false);

let timer: number | undefined;
let offDaemon: (() => void) | undefined;

const peers = computed(() => Object.values(status.value?.Peer || {}));
const connected = computed(() => status.value?.BackendState === "Running");
const currentExitNode = computed(() => status.value?.ExitNodeStatus?.ID || "");
const exitNodeOptions = computed(() =>
  peers.value
    .filter((peer) => peer.ExitNodeOption)
    .sort((a, b) => nodeName(a).localeCompare(nodeName(b), "zh-CN")),
);

onMounted(() => {
  void refresh();
  timer = window.setInterval(() => void refresh(false), 5000);
  offDaemon = window.scaletail.onDaemonEvent(() => void refresh(false));
});

onUnmounted(() => {
  if (timer) {
    window.clearInterval(timer);
  }
  offDaemon?.();
});

async function refresh(showSpinner = true) {
  if (showSpinner) {
    loading.value = true;
  }
  error.value = "";
  try {
    const [nextStatus, nextPrefs] = await Promise.all([
      window.scaletail.getStatus(true),
      window.scaletail.getPrefs(),
    ]);
    status.value = nextStatus;
    prefs.value = nextPrefs;
    if (!exitNodeTouched.value) {
      exitNodeID.value = nextStatus.ExitNodeStatus?.ID || "";
    }
    if (!routesTouched.value) {
      advertiseRoutes.value = routesFromPrefs(nextPrefs).join("\n");
    }
  } catch (err) {
    error.value = messageOf(err);
  } finally {
    loading.value = false;
  }
}

async function applyExitNode() {
  exitNodeLoading.value = true;
  error.value = "";
  message.value = "";
  try {
    await window.scaletail.setExitNode(exitNodeID.value);
    exitNodeTouched.value = false;
    message.value = "出口节点已更新。";
    await refresh(false);
  } catch (err) {
    error.value = messageOf(err);
  } finally {
    exitNodeLoading.value = false;
  }
}

async function applyAdvertiseRoutes() {
  routeLoading.value = true;
  error.value = "";
  message.value = "";
  try {
    await window.scaletail.setAdvertiseRoutes(parseRoutes(advertiseRoutes.value));
    routesTouched.value = false;
    message.value = "宣告路由已更新。";
    await refresh(false);
  } catch (err) {
    error.value = messageOf(err);
  } finally {
    routeLoading.value = false;
  }
}

async function runNetcheck() {
  netcheckLoading.value = true;
  error.value = "";
  message.value = "";
  try {
    netcheck.value = await window.scaletail.runNetcheck();
  } catch (err) {
    error.value = messageOf(err);
  } finally {
    netcheckLoading.value = false;
  }
}

function parseRoutes(raw: string) {
  return [...new Set(raw.split(/[\s,，;；]+/).map((item) => item.trim()).filter(Boolean))];
}

function clearRoutes() {
  advertiseRoutes.value = "";
  routesTouched.value = true;
}

function routesFromPrefs(nextPrefs: Prefs) {
  const raw = nextPrefs.AdvertiseRoutes;
  return Array.isArray(raw) ? raw.map(String) : [];
}

function nodeName(peer: PeerStatus) {
  return peer.HostName || (peer.DNSName ? peer.DNSName.split(".")[0] : "") || peer.ID || "-";
}

function fmtBool(value: unknown) {
  return value === true ? "是" : value === false ? "否" : "-";
}

function fmtLatency(value: unknown) {
  if (typeof value !== "number") {
    return "-";
  }
  return `${Math.round(value / 1000000)} ms`;
}

function exitNodeLabel() {
  if (!currentExitNode.value) {
    return "未使用";
  }
  const peer = peers.value.find((p) => p.ID === currentExitNode.value);
  if (!peer) {
    return currentExitNode.value;
  }
  return `${nodeName(peer)}${peer.Online === false ? "（离线）" : ""}`;
}

function messageOf(err: unknown) {
  return err instanceof Error ? err.message : String(err || "未知错误");
}
</script>

<template>
  <section class="view-stack">
    <div v-if="error" class="notice error compact">
      <strong>操作失败</strong>
      <p>{{ error }}</p>
    </div>
    <div v-if="message" class="notice ok compact">
      <strong>提示</strong>
      <p>{{ message }}</p>
    </div>

    <div class="toolbar">
      <button class="btn primary" @click="refresh()">
        <RefreshCw :size="16" :class="{ spin: loading }" />
        刷新
      </button>
      <button class="btn" @click="runNetcheck">
        <Router :size="16" :class="{ spin: netcheckLoading }" />
        运行 netcheck
      </button>
    </div>

    <div class="grid two">
      <section class="panel">
        <h2>出口节点</h2>
        <div class="exit-form">
          <select v-model="exitNodeID" @change="exitNodeTouched = true">
            <option value="">不使用出口节点</option>
            <option v-for="peer in exitNodeOptions" :key="peer.ID" :value="peer.ID" :disabled="peer.Online === false">
              {{ nodeName(peer) }}{{ peer.Online === false ? "（离线）" : "" }}
            </option>
          </select>
          <button class="btn primary" :disabled="!connected || exitNodeLoading" @click="applyExitNode">
            <Save :size="16" />
            应用
          </button>
        </div>
        <p class="hint">当前：{{ exitNodeLabel() }}</p>
      </section>

      <section class="panel">
        <h2>宣告路由</h2>
        <textarea
          v-model="advertiseRoutes"
          class="route-textarea mono"
          placeholder="192.168.10.0/24&#10;10.10.0.0/16"
          @input="routesTouched = true"
        />
        <div class="toolbar compact-actions">
          <button class="btn primary" :disabled="!connected || routeLoading" @click="applyAdvertiseRoutes">
            <Save :size="16" />
            应用
          </button>
          <button class="btn" :disabled="routeLoading" @click="clearRoutes">清空</button>
        </div>
      </section>
    </div>

    <section class="panel">
      <h2>网络检测</h2>
      <dl v-if="netcheck" class="rows">
        <dt>UDP</dt>
        <dd>{{ fmtBool(netcheck.UDP) }}</dd>
        <dt>IPv4</dt>
        <dd>{{ fmtBool(netcheck.IPv4) }}</dd>
        <dt>IPv6</dt>
        <dd>{{ fmtBool(netcheck.IPv6) }}</dd>
        <dt>首选 DERP</dt>
        <dd>{{ netcheck.PreferredDERP || "-" }}</dd>
        <dt>公网 IPv4</dt>
        <dd>{{ netcheck.GlobalV4 || "-" }}</dd>
        <dt>公网 IPv6</dt>
        <dd>{{ netcheck.GlobalV6 || "-" }}</dd>
        <template v-for="(value, key) in netcheck.RegionLatency || netcheck.DERPLatency || {}" :key="key">
          <dt>DERP {{ key }}</dt>
          <dd>{{ fmtLatency(value) }}</dd>
        </template>
      </dl>
      <p v-else class="hint">点击“运行 netcheck”查看 NAT、UDP 和 DERP 延迟。</p>
    </section>
  </section>
</template>
