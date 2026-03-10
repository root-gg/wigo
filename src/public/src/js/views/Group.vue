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

      <li v-for="host in sortedHosts" :key="host.Name" class="nav-item">
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
      v-for="host in sortedHosts"
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
                  v-for="probe in sortedProbes(host.Probes)"
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
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import api from "../api/client.js";
import { getLevel, getStatusRowClass } from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import StatusBadge from "../components/StatusBadge.vue";
import { useRefresh } from "../composables/useRefresh.js";

const route = useRoute();
const router = useRouter();
const groupName = ref(route.query.name || "");
const group = ref(null);
const hosts = ref([]);
const counts = ref({
  OK: 0,
  INFO: 0,
  WARNING: 0,
  CRITICAL: 0,
  ERROR: 0,
});

const sortedHosts = computed(() => {
  return [...hosts.value].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  });
});

function sortedProbes(probes) {
  return [...probes].sort((a, b) => {
    if (b.Status !== a.Status) return b.Status - a.Status;
    return a.Name.localeCompare(b.Name);
  });
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
  hosts.value = [];
  counts.value = {
    OK: 0,
    INFO: 0,
    WARNING: 0,
    CRITICAL: 0,
    ERROR: 0,
  };

  if (!groupName.value) return;

  try {
    const groupData = await api.getGroup(groupName.value);
    group.value = groupData;
    group.value.Level = getLevel(group.value.Status);

    for (const host of groupData.Hosts) {
      host.counts = {
        OK: 0,
        INFO: 0,
        WARNING: 0,
        CRITICAL: 0,
        ERROR: 0,
      };
      host.Level = getLevel(host.Status);

      for (const probe of host.Probes) {
        probe.Level = getLevel(probe.Status);
        counts.value[probe.Level]++;
        host.counts[probe.Level]++;
      }

      hosts.value.push(host);
    }
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

onMounted(() => {
  groupName.value = route.query.name || "";
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
