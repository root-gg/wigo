<template>
  <AppLayout
    :counts="counts"
    :current-interval="interval"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="All hosts & groups">
          <i class="fas fa-fw fa-network-wired"></i
          ><span>&nbsp;All hosts & groups </span>
        </a>
      </li>
      <li v-for="group in visibleGroups" :key="group.Name" class="nav-item">
        <a
          class="nav-link px-3 py-1 cursor-pointer"
          :title="groupTitle(group)"
          @click="gotoAnchor(group.Name)"
        >
          <i class="fas fa-fw fa-folder"></i
          ><span
            >&nbsp;{{ group.Name }}
            <StatusBadge
              v-for="(count, countName) in group.counts"
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
      v-for="group in visibleGroups"
      :key="group.Name"
      :id="group.Name"
      class="jump"
    >
      <StatusCard
        :level="group.Level"
        :clickable="true"
        @click="gotoGroup(group.Name)"
      >
        <template #title>
          {{ group.Name }}
        </template>
        <template #badges>
          <StatusBadge
            v-for="(count, countName) in group.counts"
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
                  <th>Hostname</th>
                  <th>Probes</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="host in group.visibleHosts"
                  :key="host.Name"
                  :class="getStatusRowClass(host.Status)"
                  class="cursor-pointer"
                  @click="gotoHost(host.Name)"
                >
                  <td>
                    <span>{{ host.Name }}</span>
                    <strong v-if="!host.IsAlive" class="text-danger ms-2">
                      {{ host.Message }}
                    </strong>
                  </td>
                  <td>
                    <StatusBadge
                      v-for="probe in sortedProbes(host.Probes)"
                      :key="probe.Name"
                      :level="probe.Level"
                      size="sm"
                      class="me-1 cursor-pointer"
                      :title="probe.Message"
                      @click.stop="gotoProbe(host.Name, probe.Name)"
                    >
                      {{ probe.Name }}
                    </StatusBadge>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </StatusCard>
    </div>

    <p v-if="loaded && !visibleGroups.length" class="text-body-secondary my-4">
      <i class="fas fa-filter me-2"></i>No host matches the current filter.
    </p>
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import api from "../api/client.js";
import { getLevel, getStatusRowClass } from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import StatusBadge from "../components/StatusBadge.vue";
import { useRefresh } from "../composables/useRefresh.js";
import { useDashboardFilter } from "../composables/useDashboardFilter.js";

const router = useRouter();
const groups = ref([]);
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

const sortedGroups = computed(() => [...groups.value].sort(byStatusThenName));

/**
 * Les compteurs de la vue d'accueil comptent des hosts : le filtre porte donc
 * sur les hosts, et un groupe sans host visible disparaît.
 */
const visibleGroups = computed(() =>
  sortedGroups.value
    .map((group) => ({
      ...group,
      visibleHosts: [...group.Hosts]
        .sort(byStatusThenName)
        .filter((host) =>
          matches(
            host.Level,
            host.Name,
            group.Name,
            ...host.Probes.map((probe) => probe.Name),
          ),
        ),
    }))
    .filter((group) => group.visibleHosts.length > 0),
);

function sortedProbes(probes) {
  return [...probes].sort(byStatusThenName);
}

function gotoGroup(groupName) {
  router.push({ path: "/group", query: { name: groupName } });
}

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

function groupTitle(group) {
  const parts = [group.Name];
  if (group.counts && Object.keys(group.counts).length) {
    const status = Object.entries(group.counts)
      .filter(([, v]) => v)
      .map(([k, v]) => `${k}: ${v}`)
      .join(", ");
    if (status) parts.push(status);
  }
  return parts.join(" - ");
}

async function load() {
  try {
    const groupNames = await api.getGroups();

    // Les groupes sont chargés en parallèle, et l'affichage n'est remplacé
    // qu'une fois tout arrivé : pas de cascade de requêtes ni de page vide
    // entre deux rafraîchissements.
    const results = await Promise.all(
      groupNames.map((groupName) =>
        api.getGroup(groupName).catch((error) => {
          console.error(`Error loading group ${groupName}:`, error);
          return null;
        }),
      ),
    );

    const nextCounts = emptyCounts();
    const nextGroups = [];

    for (const group of results) {
      if (!group) continue;

      group.counts = emptyCounts();
      group.Level = getLevel(group.Status);

      for (const host of group.Hosts) {
        host.Level = getLevel(host.Status);
        nextCounts[host.Level]++;
        group.counts[host.Level]++;

        for (const probe of host.Probes) {
          probe.Level = getLevel(probe.Status);
        }
      }

      nextGroups.push(group);
    }

    groups.value = nextGroups;
    counts.value = nextCounts;
    loaded.value = true;
  } catch (error) {
    console.error("Error loading hosts:", error);
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
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
