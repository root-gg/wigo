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
          :title="`${probe.Name} - ${probe.Disabled ? 'disabled' : probe.Status}`"
          @click="gotoAnchor(probe.Name)"
        >
          <i class="fas fa-fw fa-chart-line"></i
          ><span
            >&nbsp;{{ probe.Name }}
            <StatusBadge :level="probe.Level" size="sm" class="ms-1">
              {{ probe.Disabled ? "off" : probe.Status }}
            </StatusBadge>
          </span>
        </a>
      </li>
    </template>

    <div
      v-if="scheduleError"
      class="alert alert-secondary d-flex align-items-center gap-2 mt-4 mb-0 py-2"
    >
      <i class="fas fa-fw fa-circle-info"></i>
      <span>{{ scheduleError }}</span>
    </div>

    <div
      v-if="disabledCount"
      class="alert alert-secondary d-flex align-items-center gap-2 mt-4 mb-0 py-2"
    >
      <i class="fas fa-ban"></i>
      <span>
        {{ disabledCount }}
        {{ disabledCount > 1 ? "probes are" : "probe is" }} disabled on this
        host and {{ disabledCount > 1 ? "are" : "is" }} not being monitored.
      </span>
    </div>

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
        <template #badges>
          <ProbeScheduleControl
            :host-name="hostName"
            :probe-name="probe.Name"
            :schedule="scheduleOf(probe.Name)"
            :editable="canEditSchedule"
            :read-only-reason="readOnlyReason"
            @changed="onScheduleChanged"
          />
        </template>
        <template #body>
          <p v-if="probe.Disabled" class="mb-0 text-body-secondary">
            This probe is disabled: it is not executed at all, so nothing about
            it is being monitored.
          </p>
          <template v-else>
            <p class="mb-3">{{ probe.Message }}</p>
            <div v-if="probe.Detail" class="mt-3">
              <pre
                class="border rounded p-3 bg-body-tertiary"
                style="max-height: 400px; overflow: auto"
                >{{ JSON.stringify(probe.Detail, null, 2) }}</pre>
            </div>
          </template>
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
import { getLevel, DISABLED_LEVEL } from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import StatusBadge from "../components/StatusBadge.vue";
import ProbeScheduleControl from "../components/ProbeScheduleControl.vue";
import { useRefresh } from "../composables/useRefresh.js";
import { useDashboardFilter } from "../composables/useDashboardFilter.js";

const route = useRoute();
const hostName = ref(route.query.name || "");
const host = ref(null);
const probes = ref([]);
const schedule = ref(null);
const scheduleError = ref("");
const loaded = ref(false);
const counts = ref(emptyCounts());

const { matches, matchesWithoutLevel } = useDashboardFilter();

function emptyCounts() {
  return {
    OK: 0,
    INFO: 0,
    WARNING: 0,
    CRITICAL: 0,
    ERROR: 0,
  };
}

/**
 * Le master relaie la demande au host visé, donc l'ordonnancement décrit bien
 * ce host-là. On vérifie quand même qu'il se reconnaît sous ce nom.
 */
const hasSchedule = computed(
  () => !!schedule.value && schedule.value.Hostname === hostName.value,
);

const canEditSchedule = computed(
  () => hasSchedule.value && !!schedule.value.WriteActionsAllowed,
);

const readOnlyReason = computed(
  () =>
    `Read only: set AllowWriteActions in the [Http] section of the configuration file on ${hostName.value}`,
);

const scheduleByName = computed(() => {
  const byName = {};
  if (!hasSchedule.value) return byName;

  // Une probe installée à plusieurs intervalles apparaît une fois par
  // emplacement. On garde la liste : l'API refuse d'agir sur elle, l'interface
  // doit le dire plutôt que d'en afficher un au hasard.
  for (const location of schedule.value.Probes || []) {
    if (byName[location.Name]) {
      byName[location.Name].Directories.push(location.Directory);
    } else {
      byName[location.Name] = {
        ...location,
        Directories: [location.Directory],
      };
    }
  }
  return byName;
});

function scheduleOf(probeName) {
  return scheduleByName.value[probeName] || null;
}

/** Probes désactivées : elles n'ont aucun résultat, seul le disque les connaît */
const disabledProbes = computed(() => {
  if (!hasSchedule.value) return [];

  const withResult = new Set(probes.value.map((probe) => probe.Name));

  return (schedule.value.Probes || [])
    .filter((location) => !location.Enabled && !withResult.has(location.Name))
    .map((location) => ({
      Name: location.Name,
      Level: DISABLED_LEVEL,
      Disabled: true,
      Status: null,
      Message: "",
    }));
});

const disabledCount = computed(() => disabledProbes.value.length);

const sortedProbes = computed(() =>
  [...probes.value].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  }),
);

const visibleProbes = computed(() => {
  const running = sortedProbes.value.filter((probe) =>
    matches(probe.Level, probe.Name, probe.Message),
  );

  // Les désactivées ferment la liste : elles n'ont pas de statut, donc pas de
  // place dans un tri par gravité.
  const disabled = [...disabledProbes.value]
    .filter((probe) => matchesWithoutLevel(probe.Name))
    .sort((a, b) => a.Name.localeCompare(b.Name));

  return [...running, ...disabled];
});

function gotoAnchor(anchor) {
  const element = document.getElementById(anchor);
  if (element) {
    element.scrollIntoView({ behavior: "smooth" });
  }
}

function onScheduleChanged(updated) {
  schedule.value = updated;
  // Le résultat d'une probe qu'on vient de réactiver n'arrivera qu'au prochain
  // cycle, mais le reste de la page doit refléter le changement tout de suite.
  load();
}

async function loadSchedule() {
  try {
    schedule.value = await api.getHostProbesSchedule(hostName.value);
    scheduleError.value = "";
  } catch (error) {
    // Un wigo trop ancien n'a pas cet endpoint, et un host que ce master ne
    // sonde pas directement n'est pas joignable. La page reste utilisable,
    // simplement sans les contrôles, et on dit pourquoi.
    schedule.value = null;
    scheduleError.value =
      error.response?.data ||
      "The schedule of this host could not be read from here";
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

async function loadAll() {
  await Promise.all([load(), loadSchedule()]);
}

const { startRefresh, stopRefresh, setRefreshInterval, interval } = useRefresh(
  loadAll,
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
    loadAll();
  },
);

onMounted(() => {
  hostName.value = route.query.name || "";
  loadAll();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
