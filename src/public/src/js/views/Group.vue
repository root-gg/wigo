<template>
  <AppLayout
    :counts="counts"
    :current-interval="interval"
    :title-context="groupName"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a
          class="nav-link px-3 py-1"
          :title="
            group
              ? `Group ${groupName} - ${group.Status}`
              : `Group ${groupName}`
          "
        >
          <i class="fas fa-fw fa-folder"></i
          ><span
            >&nbsp;Group {{ groupName }}
            <StatusBadge
              v-if="group"
              :level="group.Level"
              size="sm"
              class="ms-1"
            >
              {{ group.Status }}
            </StatusBadge>
          </span>
        </a>
      </li>

      <li v-for="host in visibleHosts" :key="host.Name" class="nav-item">
        <a
          class="nav-link px-3 py-1 cursor-pointer"
          :title="hostTitle(host)"
          @click="gotoAnchor(host.Name)"
        >
          <i class="fas fa-fw fa-server"></i
          ><span
            >&nbsp;{{ host.Name }}
            <small v-if="!host.IsAlive" class="text-danger ms-1">
              {{ host.Message }}
            </small>
            <StatusBadge
              v-for="(count, countName) in host.counts"
              :key="countName"
              :level="countName"
              size="sm"
              class="ms-1"
              v-show="count"
            >
              {{ count }}
            </StatusBadge>
          </span>
        </a>
      </li>
    </template>

    <div
      v-for="host in visibleHosts"
      :key="host.Name"
      :id="host.Name"
      class="jump"
    >
      <StatusCard
        :level="host.Level"
        :clickable="true"
        @click="gotoHost(host.Name)"
      >
        <template #title>
          {{ host.Name }}
        </template>
        <template #badges>
          <StatusBadge
            v-for="(count, countName) in host.counts"
            :key="countName"
            :level="countName"
            size="sm"
            v-show="count"
          >
            {{ count }}
          </StatusBadge>
        </template>
        <template #body>
          <div class="table-responsive">
            <table class="table table-bordered table-hover">
              <thead>
                <tr>
                  <th>Probe</th>
                  <th>Message</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="probe in host.visibleProbes"
                  :key="probe.Name"
                  :class="getStatusRowClass(probe.Status)"
                  class="cursor-pointer"
                  @click="gotoProbe(host.Name, probe.Name)"
                >
                  <td>{{ probe.Name }}</td>
                  <td>{{ probe.Message }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </StatusCard>
    </div>

    <p v-if="loaded && !visibleHosts.length" class="text-body-secondary my-4">
      <i class="fas fa-filter me-2"></i>No probe matches the current filter.
    </p>
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import api from "../api/client.js";
import { getLevel, getStatusRowClass } from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import StatusBadge from "../components/StatusBadge.vue";
import { useRefresh } from "../composables/useRefresh.js";
import { useLiveEvents } from "../composables/useLiveEvents.js";
import { useDashboardFilter } from "../composables/useDashboardFilter.js";

const route = useRoute();
const router = useRouter();
const groupName = ref(route.query.name || "");
const group = ref(null);
const hosts = ref([]);
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

function byStatusThenName(a, b) {
  if (b.Status !== a.Status) return b.Status - a.Status;
  return a.Name.localeCompare(b.Name);
}

const sortedHosts = computed(() => [...hosts.value].sort(byStatusThenName));

/**
 * Les compteurs de la vue groupe comptent des probes : le filtre porte donc
 * sur les probes, et un host sans probe visible disparaît.
 */
const visibleHosts = computed(() =>
  sortedHosts.value
    .map((host) => ({
      ...host,
      visibleProbes: [...host.Probes]
        .sort(byStatusThenName)
        .filter((probe) =>
          matches(probe.Level, probe.Name, probe.Message, host.Name),
        ),
    }))
    .filter((host) => host.visibleProbes.length > 0),
);

function gotoHost(hostName) {
  router.push({ path: "/host", query: { name: hostName } });
}

function gotoProbe(hostName, probeName) {
  router.push({
    path: "/host",
    query: { name: hostName },
    hash: `#${probeName}`,
  });
}

function gotoAnchor(anchor) {
  const element = document.getElementById(anchor);
  if (element) {
    element.scrollIntoView({ behavior: "smooth" });
  }
}

function hostTitle(host) {
  const parts = [host.Name];
  if (!host.IsAlive && host.Message) parts.push(host.Message);
  if (host.counts && Object.keys(host.counts).length) {
    const status = Object.entries(host.counts)
      .filter(([, v]) => v)
      .map(([k, v]) => `${k}: ${v}`)
      .join(", ");
    if (status) parts.push(status);
  }
  return parts.join(" - ");
}

async function load() {
  if (!groupName.value) return;

  try {
    const groupData = await api.getGroup(groupName.value);
    groupData.Level = getLevel(groupData.Status);

    const nextCounts = emptyCounts();
    const nextHosts = [];

    for (const host of groupData.Hosts) {
      host.counts = emptyCounts();
      host.Level = getLevel(host.Status);

      for (const probe of host.Probes) {
        probe.Level = getLevel(probe.Status);
        nextCounts[probe.Level]++;
        host.counts[probe.Level]++;
      }

      nextHosts.push(host);
    }

    group.value = groupData;
    hosts.value = nextHosts;
    counts.value = nextCounts;
    loaded.value = true;
  } catch (error) {
    console.error("Error loading group:", error);
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
// (/group?name=a -> /group?name=b) : il faut recharger explicitement.
watch(
  () => route.query.name,
  (name) => {
    groupName.value = name || "";
    hosts.value = [];
    group.value = null;
    loaded.value = false;
    counts.value = emptyCounts();
    load();
  },
);

// Le flux donne l'immédiateté, le rafraîchissement périodique reste le filet
useLiveEvents(load);

onMounted(() => {
  groupName.value = route.query.name || "";
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
