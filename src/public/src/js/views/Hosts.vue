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
      <li v-for="group in sortedGroups" :key="group.Name" class="nav-item">
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
      v-for="group in sortedGroups"
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
                  v-for="host in sortedHosts(group.Hosts)"
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

const router = useRouter();
const groups = ref([]);
const counts = ref({
  OK: 0,
  INFO: 0,
  WARNING: 0,
  CRITICAL: 0,
  ERROR: 0,
});

const sortedGroups = computed(() => {
  return [...groups.value].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  });
});

function sortedHosts(hosts) {
  return [...hosts].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  });
}

function sortedProbes(probes) {
  return [...probes].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  });
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
  groups.value = [];
  counts.value = {
    OK: 0,
    INFO: 0,
    WARNING: 0,
    CRITICAL: 0,
    ERROR: 0,
  };

  try {
    const groupNames = await api.getGroups();

    for (const groupName of groupNames) {
      const group = await api.getGroup(groupName);

      group.counts = {
        OK: 0,
        INFO: 0,
        WARNING: 0,
        CRITICAL: 0,
        ERROR: 0,
      };
      group.Level = getLevel(group.Status);

      for (const host of group.Hosts) {
        host.Level = getLevel(host.Status);
        counts.value[host.Level]++;
        group.counts[host.Level]++;

        for (const probe of host.Probes) {
          probe.Level = getLevel(probe.Status);
        }
      }

      groups.value.push(group);
    }
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
