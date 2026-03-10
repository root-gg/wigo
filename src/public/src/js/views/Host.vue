<template>
  <AppLayout
    :counts="counts"
    :current-interval="interval"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a
          class="nav-link px-3 py-1"
          :title="
            host
              ? `Host ${hostName} - ${host.GlobalStatus}`
              : `Host ${hostName}`
          "
        >
          <i class="fas fa-fw fa-server"></i
          ><span
            >&nbsp;Host {{ hostName }}
            <small
              v-if="host && !host.IsAlive"
              class="text-danger d-block mt-1"
            >
              {{ host.GlobalMessage }}
            </small>
            <StatusBadge
              v-if="host"
              :level="host.GlobalLevel"
              size="sm"
              class="ms-1"
            >
              {{ host.GlobalStatus }}
            </StatusBadge>
          </span>
        </a>
      </li>

      <li v-for="probe in sortedProbes" :key="probe.Name" class="nav-item">
        <a
          class="nav-link px-3 py-1 cursor-pointer"
          :title="`${probe.Name} - ${probe.Status}`"
          @click="gotoAnchor(probe.Name)"
        >
          <i class="fas fa-fw fa-chart-line"></i
          ><span
            >&nbsp;{{ probe.Name }}
            <StatusBadge :level="probe.Level" size="sm" class="ms-1">
              {{ probe.Status }}
            </StatusBadge>
          </span>
        </a>
      </li>
    </template>

    <div
      v-for="probe in sortedProbes"
      :key="probe.Name"
      :id="probe.Name"
      class="jump"
    >
      <StatusCard :level="probe.Level">
        <template #title>
          <strong>{{ probe.Name }}</strong>
        </template>
        <template #body>
          <p class="mb-3">{{ probe.Message }}</p>
          <div v-if="probe.Detail" class="mt-3">
            <pre
              class="border rounded p-3 bg-light"
              style="max-height: 400px; overflow: auto"
              >{{ JSON.stringify(probe.Detail, null, 2) }}</pre
            >
          </div>
        </template>
      </StatusCard>
    </div>
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import api from "../api/client.js";
import { getLevel } from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import StatusBadge from "../components/StatusBadge.vue";
import { useRefresh } from "../composables/useRefresh.js";

const route = useRoute();
const router = useRouter();
const hostName = ref(route.query.name || "");
const host = ref(null);
const probes = ref([]);
const counts = ref({
  OK: 0,
  INFO: 0,
  WARNING: 0,
  CRITICAL: 0,
  ERROR: 0,
});

const sortedProbes = computed(() => {
  return [...probes.value].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  });
});

function gotoAnchor(anchor) {
  const element = document.getElementById(anchor);
  if (element) {
    element.scrollIntoView({ behavior: "smooth" });
  }
}

async function load() {
  probes.value = [];
  counts.value = {
    OK: 0,
    INFO: 0,
    WARNING: 0,
    CRITICAL: 0,
    ERROR: 0,
  };

  if (!hostName.value) return;

  try {
    const hostData = await api.getHost(hostName.value);
    host.value = hostData;
    host.value.LocalHost.Level = getLevel(host.value.LocalHost.Status);
    host.value.GlobalLevel = getLevel(host.value.GlobalStatus);

    // Convert Probes from object (map) to array if needed
    let probesArray = hostData.LocalHost?.Probes;
    if (!probesArray) {
      probesArray = [];
    } else if (!Array.isArray(probesArray)) {
      // If Probes is an object (map), convert it to an array
      probesArray = Object.values(probesArray);
    }

    for (const probe of probesArray) {
      probe.Level = getLevel(probe.Status);
      counts.value[probe.Level]++;
      probes.value.push(probe);
    }

    // Scroll to anchor if hash is present
    if (route.hash) {
      setTimeout(() => {
        gotoAnchor(route.hash.substring(1));
      }, 100);
    }
  } catch (error) {
    console.error("Error loading host:", error);
  }
}

const { startRefresh, stopRefresh, setRefreshInterval, interval } = useRefresh(
  load,
  60,
);

function handleRefreshSettings(seconds) {
  setRefreshInterval(seconds);
}

onMounted(() => {
  hostName.value = route.query.name || "";
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
