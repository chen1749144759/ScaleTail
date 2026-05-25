<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  state?: string;
}>();

const label = computed(() => {
  const map: Record<string, string> = {
    Running: "已连接",
    Starting: "正在连接",
    NeedsLogin: "需要认证",
    NeedsMachineAuth: "等待授权",
    NoState: "未配置",
    Stopped: "已断开",
  };
  return map[props.state || ""] || props.state || "状态未知";
});
</script>

<template>
  <span class="status-pill" :class="state || 'unknown'">
    <i />
    {{ label }}
  </span>
</template>
