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
          :title="probeTitle(probe)"
          @click="gotoAnchor(probe.Name)"
        >
          <i class="fas fa-fw fa-chart-line"></i
          ><span
            >&nbsp;{{ probe.Name }}
            <StatusBadge :level="probe.Level" size="sm" class="ms-1">
              {{ probeBadge(probe) }}
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
            on-color
            :schedule="scheduleOf(probe.Name)"
            :editable="canEditSchedule"
            :read-only-reason="readOnlyReason"
            @changed="onScheduleChanged"
            @rechecked="load"
          />
        </template>
        <template #body>
          <!-- Elle a tourné et a demandé qu'on la laisse tranquille : ce n'est
               ni une panne ni une attente, et le seul geste utile est de la
               désactiver pour de bon si la machine ne la concerne pas. -->
          <p v-if="probe.Skipped" class="mb-0 text-body-secondary">
            <i class="fas fa-fw fa-circle-info"></i>
            This probe ran and asked not to be run again, which usually means
            there is nothing for it to check on this host. It is still scheduled
            every {{ formatInterval(probe.Interval) }}; recheck it to give it
            another chance without restarting wigo.
          </p>

          <!-- Elle vient d'être activée : le résultat n'arrivera qu'au premier
               passage, et la carte doit le dire plutôt que de disparaître de la
               page en attendant. -->
          <p v-else-if="probe.Pending" class="mb-0 text-body-secondary">
            <i class="fas fa-fw fa-hourglass-half"></i>
            This probe is now scheduled every
            {{ formatInterval(probe.Interval) }}. It has not run yet, so its
            first result will appear on the next cycle.
          </p>
          <div v-else-if="probe.Disabled" class="text-body-secondary">
            <p class="mb-0">
              This probe is disabled: nothing schedules it, so it is never
              executed and nothing about it is being monitored.
            </p>

            <!-- Quelqu'un l'a décidé. Le reste des sondes désactivées n'a
                 jamais été activé, ce qui ne s'attribue à personne. -->
            <p v-if="disableRecordOf(probe.Name)" class="mb-0 mt-2">
              <i class="fas fa-fw fa-user-clock"></i>
              {{ describeDisable(disableRecordOf(probe.Name)) }}
              <span class="d-block ms-4 ps-1">
                {{ describeExpiry(disableRecordOf(probe.Name)) }}
              </span>
            </p>
          </div>
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
import {
  disableRecordsByProbe,
  describeDisable,
  describeExpiry,
} from "../utils/disable.js";

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

/**
 * Les probes sans résultat, que seul l'ordonnancement sur disque connaît. Trois
 * raisons de ne pas en avoir, qui se ressemblent de loin et n'appellent pas du
 * tout la même chose :
 *
 * - rien ne les ordonnance, elles sont désactivées ;
 * - elles ont tourné et sont sorties en code 13, demandant à ne plus l'être ;
 * - elles viennent d'être activées et n'ont pas encore tourné.
 *
 * La dernière est celle qui manquait : sans elle, activer une probe la faisait
 * disparaître de la page jusqu'à sa première exécution, soit une heure entière
 * si c'est l'intervalle qu'on vient de choisir. Et sans la deuxième, une probe
 * comme check_mdadm sur une machine sans grappe RAID restait éternellement
 * annoncée comme sur le point de tourner.
 */
const disableRecords = computed(() =>
  hasSchedule.value ? disableRecordsByProbe(schedule.value) : {},
);

function disableRecordOf(probeName) {
  return disableRecords.value[probeName] || null;
}

const skippedProbes = computed(
  () => new Set(hasSchedule.value ? schedule.value.SkippedProbes || [] : []),
);

const probesWithoutResult = computed(() => {
  if (!hasSchedule.value) return [];

  const withResult = new Set(probes.value.map((probe) => probe.Name));

  return (schedule.value.Probes || [])
    .filter((location) => !withResult.has(location.Name))
    .map((location) => ({
      Name: location.Name,
      Level: DISABLED_LEVEL,
      Disabled: !location.Enabled,
      Skipped: location.Enabled && skippedProbes.value.has(location.Name),
      Pending: location.Enabled && !skippedProbes.value.has(location.Name),
      Interval: location.Interval,
      Status: null,
      Message: "",
    }));
});

const disabledCount = computed(
  () => probesWithoutResult.value.filter((probe) => probe.Disabled).length,
);

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

  // Les sans-résultat ferment la liste : elles n'ont pas de statut, donc pas de
  // place dans un tri par gravité. Celles qui attendent leur première exécution
  // passent devant les désactivées, on vient de demander qu'elles tournent.
  const withoutResult = [...probesWithoutResult.value]
    .filter((probe) => matchesWithoutLevel(probe.Name))
    .sort((a, b) => {
      if (a.Pending !== b.Pending) return a.Pending ? -1 : 1;
      if (a.Skipped !== b.Skipped) return a.Skipped ? -1 : 1;
      return a.Name.localeCompare(b.Name);
    });

  return [...running, ...withoutResult];
});

function formatInterval(seconds) {
  if (!seconds) return "—";
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}min`;
  return `${seconds}s`;
}

function probeBadge(probe) {
  if (probe.Skipped) return "n/a";
  if (probe.Pending) return "…";
  if (probe.Disabled) return "off";
  return probe.Status;
}

function probeTitle(probe) {
  if (probe.Skipped) {
    return `${probe.Name} - ran and asked not to be run again, recheck it to try again`;
  }
  if (probe.Pending) {
    return `${probe.Name} - scheduled every ${formatInterval(probe.Interval)}, waiting for its first result`;
  }
  return `${probe.Name} - ${probe.Disabled ? "disabled" : probe.Status}`;
}

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
