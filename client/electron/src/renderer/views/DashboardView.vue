<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { RefreshCw, Wrench } from "lucide-vue-next";
import type { PeerStatus, ServiceOverview, Status } from "../../shared/types";

const status = ref<Status | null>(null);
const service = ref<ServiceOverview | null>(null);
const loading = ref(false);
const error = ref("");

let timer: number | undefined;
let offDaemon: (() => void) | undefined;

const peers = computed(() => Object.values(status.value?.Peer || {}));
const selfName = computed(() => status.value?.Self?.HostName || status.value?.Self?.DNSName?.split(".")[0] || "-");
const selfIPs = computed(() => (status.value?.ScaleTailIPs || status.value?.Self?.ScaleTailIPs || []).join(", ") || "-");
const totalRx = computed(() => peers.value.reduce((sum, p) => sum + Number(p.RxBytes || 0), 0));
const totalTx = computed(() => peers.value.reduce((sum, p) => sum + Number(p.TxBytes || 0), 0));
const onlineCount = computed(() => peers.value.filter((peer) => peer.Online).length);

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
    const [nextStatus, nextService] = await Promise.all([
      window.scaletail.getStatus(true),
      window.scaletail.getServiceStatus(),
    ]);
    status.value = nextStatus;
    service.value = nextService;
  } catch (err) {
    error.value = messageOf(err);
    try {
      service.value = await window.scaletail.getServiceStatus();
    } catch {
      service.value = null;
    }
  } finally {
    loading.value = false;
  }
}

async function startService() {
  loading.value = true;
  error.value = "";
  try {
    service.value = await window.scaletail.startService();
    await refresh(false);
  } catch (err) {
    error.value = messageOf(err);
  } finally {
    loading.value = false;
  }
}

function nodeName(peer: PeerStatus) {
  return peer.HostName || (peer.DNSName ? peer.DNSName.split(".")[0] : "") || peer.ID || "-";
}

function nodeIP(peer: PeerStatus) {
  return (peer.ScaleTailIPs || []).join(", ") || "-";
}

function fmtBytes(value?: number) {
  let n = Number(value || 0);
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i === 0 || n >= 10 ? 0 : 1)} ${units[i]}`;
}

function messageOf(err: unknown) {
  return err instanceof Error ? err.message : String(err || "未知错误");
}
</script>

<template>
  <section class="view-stack">
    <div v-if="error" class="notice error">
      <div>
        <strong>无法完成操作</strong>
        <p>{{ error }}</p>
      </div>
      <button class="btn" @click="startService">
        <Wrench :size="16" />
        启动服务
      </button>
    </div>

    <div class="summary-grid">
      <article class="metric">
        <span>当前设备</span>
        <strong>{{ selfName }}</strong>
      </article>
      <article class="metric">
        <span>自己 IP</span>
        <strong class="mono">{{ selfIPs }}</strong>
      </article>
      <article class="metric">
        <span>节点</span>
        <strong>{{ onlineCount }} / {{ peers.length }}</strong>
      </article>
      <article class="metric">
        <span>节点总流量</span>
        <strong>{{ fmtBytes(totalRx) }} / {{ fmtBytes(totalTx) }}</strong>
      </article>
    </div>

    <div class="toolbar">
      <button class="btn primary" @click="refresh()">
        <RefreshCw :size="16" :class="{ spin: loading }" />
        刷新
      </button>
    </div>

    <section class="panel">
      <h2>节点</h2>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>状态</th>
              <th>名称</th>
              <th>IP</th>
              <th>流量</th>
              <th>连接</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="peer in peers" :key="peer.ID || peer.PublicKey || nodeName(peer)">
              <td>
                <span class="mini-pill" :class="peer.Online ? 'ok' : 'muted'">{{ peer.Online ? "在线" : "离线" }}</span>
              </td>
              <td>{{ nodeName(peer) }}</td>
              <td class="mono">{{ nodeIP(peer) }}</td>
              <td>{{ fmtBytes(peer.RxBytes) }} / {{ fmtBytes(peer.TxBytes) }}</td>
              <td>{{ peer.CurAddr || peer.Relay || "-" }}</td>
            </tr>
            <tr v-if="!peers.length">
              <td colspan="5" class="empty">暂无节点数据</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </section>
</template>
