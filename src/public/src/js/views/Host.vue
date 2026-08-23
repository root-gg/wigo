<template>
  <AppLayout
    :counts="counts"
    :current-interval="interval"
    :title-context="hostName"
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

      <li v-for="probe in visibleProbes" :key="probe.Name" class="nav-item">
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
      v-for="probe in visibleProbes"
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
              class="border rounded p-3 bg-body-tertiary"
              style="max-height: 400px; overflow: auto"
              >{{ JSON.stringify(probe.Detail, null, 2) }}</pre>
          </div>
        </template>
      </StatusCard>
    </div>

    <p v-if="loaded && !visibleProbes.length" class="text-body-secondary my-4">
      <i class="fas fa-filter me-2"></i>No probe matches the current filter.
    </p>
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { useRoute } from "vue-router";
import api from "../api/client.js";
import { getLevel } from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import StatusBadge from "../components/StatusBadge.vue";
import { useRefresh } from "../composables/useRefresh.js";
import { useDashboardFilter } from "../composables/useDashboardFilter.js";

const route = useRoute();
const hostName = ref(route.query.name || "");
const host = ref(null);
const probes = ref([]);
const loaded = ref(false);
const counts = ref(emptyCounts());

const { matches } = useDashboardFilter();

function emptyCounts() {
  return {
    OK: 0,
    INFO: 0,
    WARNING: 0,
    CRITICAL: 0,
    ERROR: 0,
  };
}

const sortedProbes = computed(() =>
  [...probes.value].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  }),
);

const visibleProbes = computed(() =>
  sortedProbes.value.filter((probe) =>
    matches(probe.Level, probe.Name, probe.Message),
  ),
);

function gotoAnchor(anchor) {
  const element = document.getElementById(anchor);
  if (element) {
    element.scrollIntoView({ behavior: "smooth" });
  }
}

async function load() {
  if (!hostName.value) return;

  try {
    const hostData = await api.getHost(hostName.value);
    hostData.LocalHost.Level = getLevel(hostData.LocalHost.Status);
    hostData.GlobalLevel = getLevel(hostData.GlobalStatus);

    // Probes peut arriver sous forme de tableau ou de map selon la source
    let probesArray = hostData.LocalHost?.Probes;
    if (!probesArray) {
      probesArray = [];
    } else if (!Array.isArray(probesArray)) {
      probesArray = Object.values(probesArray);
    }

    const nextCounts = emptyCounts();
    const nextProbes = [];

    for (const probe of probesArray) {
      probe.Level = getLevel(probe.Status);
      nextCounts[probe.Level]++;
      nextProbes.push(probe);
    }

    host.value = hostData;
    probes.value = nextProbes;
    counts.value = nextCounts;
    loaded.value = true;

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

// Le composant n'est pas remonté quand seul le paramètre change
// (/host?name=a -> /host?name=b) : il faut recharger explicitement.
watch(
  () => route.query.name,
  (name) => {
    hostName.value = name || "";
    probes.value = [];
    host.value = null;
    loaded.value = false;
    counts.value = emptyCounts();
    load();
  },
);

onMounted(() => {
  hostName.value = route.query.name || "";
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
