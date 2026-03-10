<template>
  <div class="d-flex flex-nowrap" style="min-height: 100vh">
    <Sidebar :collapsed="sidebarCollapsed" @toggle="toggleSidebar">
      <slot name="sidebar"></slot>
    </Sidebar>

    <div class="flex-grow-1" style="background-color: #ddd">
      <TopBar
        :counts="counts"
        :sidebar-collapsed="sidebarCollapsed"
        :current-interval="currentInterval"
        @refresh-settings="handleRefreshSettings"
      />

      <main
        class="content-wrapper"
        :class="{ 'sidebar-collapsed': sidebarCollapsed }"
      >
        <div class="container-fluid">
          <slot></slot>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import Sidebar from "./Sidebar.vue";
import TopBar from "./TopBar.vue";

const props = defineProps({
  counts: {
    type: Object,
    default: () => ({
      OK: 0,
      INFO: 0,
      WARNING: 0,
      CRITICAL: 0,
      ERROR: 0,
    }),
  },
  currentInterval: {
    type: Number,
    default: 0,
  },
});

const emit = defineEmits(["refresh-settings"]);

const windowWidth = ref(window.innerWidth);

/** Sidebar collapsed by default if the screen is small (< 992px) */
const SIDEBAR_WIDE_BREAKPOINT = 992;
const sidebarCollapsed = ref(window.innerWidth < SIDEBAR_WIDE_BREAKPOINT);

function handleResize() {
  windowWidth.value = window.innerWidth;
}

onMounted(() => {
  window.addEventListener("resize", handleResize);
});

onUnmounted(() => {
  window.removeEventListener("resize", handleResize);
});

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value;
}

function handleRefreshSettings(seconds) {
  emit("refresh-settings", seconds);
}
</script>
