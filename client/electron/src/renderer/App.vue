<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { Network, Settings, X } from "lucide-vue-next";
import StatusPill from "./components/StatusPill.vue";
import DashboardView from "./views/DashboardView.vue";
import ConnectView from "./views/ConnectView.vue";
import NodesView from "./views/NodesView.vue";
import type { Status } from "../shared/types";

type Route = "dashboard" | "connect" | "nodes";

const route = ref<Route>(routeFromHash());
const status = ref<Status | null>(null);

const title = computed(() => {
  if (route.value === "connect") {
    return "服务端设置";
  }
  if (route.value === "nodes") {
    return "节点";
  }
  return "仪表盘";
});
const subtitle = computed(() => {
  if (route.value === "connect") {
    return "连接与退出网络";
  }
  if (route.value === "nodes") {
    return "网络检测与路由";
  }
  return "设备状态";
});

let offNavigate: (() => void) | undefined;
let offDaemon: (() => void) | undefined;
let timer: number | undefined;

onMounted(() => {
  window.addEventListener("hashchange", syncFromHash);
  offNavigate = window.tailscale.onNavigate((next) => setRoute(next));
  offDaemon = window.tailscale.onDaemonEvent(() => void refreshStatus());
  timer = window.setInterval(() => void refreshStatus(), 5000);
  void refreshStatus();
});

onUnmounted(() => {
  window.removeEventListener("hashchange", syncFromHash);
  offNavigate?.();
  offDaemon?.();
  if (timer) {
    window.clearInterval(timer);
  }
});

function setRoute(next: Route) {
  route.value = next;
  const hash = `#/${next}`;
  if (window.location.hash !== hash) {
    window.location.hash = hash;
  }
}

function syncFromHash() {
  route.value = routeFromHash();
}

async function refreshStatus() {
  try {
    status.value = await window.tailscale.getStatus(false);
  } catch {
    status.value = null;
  }
}

function closeWindow() {
  void window.tailscale.closeWindow();
}

function routeFromHash(): Route {
  if (window.location.hash.includes("connect")) {
    return "connect";
  }
  if (window.location.hash.includes("nodes")) {
    return "nodes";
  }
  return "dashboard";
}
</script>

<template>
  <div class="app-shell">
    <header class="chrome-bar">
      <button class="brand-button" title="打开仪表盘" @click="setRoute('dashboard')">
        <img src="../../resources/app.ico" alt="" />
        <span>
          <strong>ScaleTail</strong>
          <small>Windows 客户端</small>
        </span>
      </button>

      <div class="page-title">
        <span class="eyebrow">{{ subtitle }}</span>
        <h1>{{ title }}</h1>
      </div>

      <div class="chrome-actions">
        <StatusPill class="top-status" :state="status?.BackendState" />
        <button class="icon-btn route-btn" :class="{ active: route === 'connect' }" title="服务端设置" @click="setRoute('connect')">
          <Settings :size="18" />
        </button>
        <button class="icon-btn route-btn" :class="{ active: route === 'nodes' }" title="节点" @click="setRoute('nodes')">
          <Network :size="18" />
        </button>
        <button class="window-close" title="关闭窗口" @click="closeWindow">
          <X :size="18" />
        </button>
      </div>
    </header>

    <main class="main">
      <DashboardView v-if="route === 'dashboard'" />
      <ConnectView v-else-if="route === 'connect'" @open-dashboard="setRoute('dashboard')" />
      <NodesView v-else />
    </main>
  </div>
</template>
