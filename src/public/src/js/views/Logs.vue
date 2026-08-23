<template>
  <AppLayout
    :current-interval="interval"
    :filterable="false"
    title-context="Logs"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Logs management">
          <i class="fas fa-fw fa-list"></i><span>&nbsp;Logs management</span>
        </a>
      </li>

      <li class="nav-item px-2 pb-2">
        <div class="input-group">
          <input
            type="text"
            v-model="menu.search"
            class="form-control form-control-sm"
            placeholder="Search..."
            @keyup.enter="load"
          />
          <button
            class="btn btn-secondary btn-sm"
            type="button"
            @click="load"
            title="Search"
          >
            <i class="fas fa-search"></i>
          </button>
        </div>
      </li>

      <li v-if="menu.group" class="nav-item">
        <span class="nav-link px-3 py-1" :title="`Group: ${menu.group}`">
          <i class="fas fa-fw fa-folder"></i
          ><span>&nbsp;Group: {{ menu.group }}</span>
          <i
            class="fas fa-times cursor-pointer text-danger ms-auto"
            @click="removeGroup"
          ></i>
        </span>
      </li>

      <li v-if="menu.host" class="nav-item">
        <span class="nav-link px-3 py-1" :title="`Host: ${menu.host}`">
          <i class="fas fa-fw fa-server"></i
          ><span>&nbsp;Host: {{ menu.host }}</span>
          <i
            class="fas fa-times cursor-pointer text-danger ms-auto"
            @click="removeHost"
          ></i>
        </span>
      </li>

      <li v-if="menu.probe" class="nav-item">
        <span class="nav-link px-3 py-1" :title="`Probe: ${menu.probe}`">
          <i class="fas fa-fw fa-chart-line"></i
          ><span>&nbsp;Probe: {{ menu.probe }}</span>
          <i
            class="fas fa-times cursor-pointer text-danger ms-auto"
            @click="removeProbe"
          ></i>
        </span>
      </li>

      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Level">
          <i class="fas fa-fw fa-signal"></i><span>&nbsp;Level</span>
        </a>
      </li>
      <li class="nav-item px-2 pb-2">
        <select
          v-model="menu.level"
          class="form-control form-control-sm"
          @change="load"
        >
          <option v-for="level in logLevels" :key="level" :value="level">
            {{ level }}
          </option>
        </select>
      </li>

      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Group">
          <i class="fas fa-fw fa-folder"></i><span>&nbsp;Group</span>
        </a>
      </li>
      <li class="nav-item px-2 pb-2">
        <div class="input-group">
          <select
            v-model="menu.group_select"
            class="form-control form-control-sm"
          >
            <option value="">-- Select --</option>
            <option v-for="group in indexes.groups" :key="group" :value="group">
              {{ group }}
            </option>
          </select>
          <button
            class="btn btn-success btn-sm"
            type="button"
            title="Add group"
            @click="setGroup(menu.group_select)"
          >
            <i class="fas fa-plus"></i>
          </button>
        </div>
      </li>

      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Host">
          <i class="fas fa-fw fa-server"></i><span>&nbsp;Host</span>
        </a>
      </li>
      <li class="nav-item px-2 pb-2">
        <div class="input-group">
          <select
            v-model="menu.host_select"
            class="form-control form-control-sm"
          >
            <option value="">-- Select --</option>
            <option v-for="host in indexes.hosts" :key="host" :value="host">
              {{ host }}
            </option>
          </select>
          <button
            class="btn btn-success btn-sm"
            type="button"
            title="Add host"
            @click="setHost(menu.host_select)"
          >
            <i class="fas fa-plus"></i>
          </button>
        </div>
      </li>

      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Probe">
          <i class="fas fa-fw fa-chart-line"></i><span>&nbsp;Probe</span>
        </a>
      </li>
      <li class="nav-item px-2 pb-2">
        <div class="input-group">
          <select
            v-model="menu.probe_select"
            class="form-control form-control-sm"
          >
            <option value="">-- Select --</option>
            <option v-for="probe in indexes.probes" :key="probe" :value="probe">
              {{ probe }}
            </option>
          </select>
          <button
            class="btn btn-success btn-sm"
            type="button"
            title="Add probe"
            @click="setProbe(menu.probe_select)"
          >
            <i class="fas fa-plus"></i>
          </button>
        </div>
      </li>
    </template>

    <div class="card my-4">
      <div class="card-header">
        <strong>Logs</strong>
      </div>
      <div class="card-body">
        <div class="table-responsive">
          <table class="table table-bordered table-hover">
            <thead>
              <tr>
                <th>Date</th>
                <th>Group</th>
                <th>Host</th>
                <th>Probe</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="log in filteredLogs"
                :key="log.id"
                :class="getLogRowClass(log.Level)"
              >
                <td>{{ formatDate(log.Timestamp) }}</td>
                <td class="cursor-pointer" @click="setGroup(log.Group)">
                  <span
                    class="cursor-pointer"
                    @click.stop="gotoGroup(log.Group)"
                  >
                    {{ log.Group }}
                  </span>
                </td>
                <td class="cursor-pointer" @click="setHost(log.Host)">
                  <span class="cursor-pointer" @click.stop="gotoHost(log.Host)">
                    {{ log.Host }}
                  </span>
                </td>
                <td class="cursor-pointer" @click="setProbe(log.Probe)">
                  <span
                    class="cursor-pointer"
                    @click.stop="gotoProbe(log.Host, log.Probe)"
                  >
                    {{ log.Probe }}
                  </span>
                </td>
                <td>{{ log.Message }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="d-flex justify-content-end mt-3">
          <button
            class="btn btn-primary btn-sm me-2"
            :disabled="offset < limit"
            @click="prev"
          >
            <i class="fas fa-chevron-left"></i> Prev
          </button>
          <button class="btn btn-primary btn-sm" @click="next">
            Next <i class="fas fa-chevron-right"></i>
          </button>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import api from "../api/client.js";
import {
  LOG_LEVELS,
  filterLogsByLevel,
  getLogRowClass,
} from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import { useRefresh } from "../composables/useRefresh.js";

const route = useRoute();
const router = useRouter();
const logLevels = LOG_LEVELS;
const logs = ref([]);
const indexes = ref({ groups: [], hosts: [], probes: [] });
const menu = ref({
  level: "OK",
  group: null,
  host: null,
  probe: null,
  search: "",
  group_select: "",
  host_select: "",
  probe_select: "",
});
const offset = ref(0);
const limit = ref(100);

const filteredLogs = computed(() => {
  let filtered = [...logs.value];

  if (menu.value.group) {
    filtered = filtered.filter((log) => log.Group === menu.value.group);
  }
  if (menu.value.host) {
    filtered = filtered.filter((log) => log.Host === menu.value.host);
  }
  if (menu.value.probe) {
    filtered = filtered.filter((log) => log.Probe === menu.value.probe);
  }
  if (menu.value.search) {
    const search = menu.value.search.toLowerCase();
    filtered = filtered.filter(
      (log) =>
        log.Message.toLowerCase().includes(search) ||
        log.Group?.toLowerCase().includes(search) ||
        log.Host?.toLowerCase().includes(search) ||
        log.Probe?.toLowerCase().includes(search),
    );
  }

  filtered = filterLogsByLevel(filtered, menu.value.level);

  return filtered.sort((a, b) => b.Timestamp - a.Timestamp);
});

function formatDate(timestamp) {
  return new Date(timestamp * 1000).toLocaleString();
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

function removeGroup() {
  menu.value.group = null;
  updateUrl();
  load();
}

function removeHost() {
  menu.value.host = null;
  updateUrl();
  load();
}

function removeProbe() {
  menu.value.probe = null;
  updateUrl();
  load();
}

function setGroup(group) {
  if (!group) return;
  menu.value.group = group;
  updateUrl();
  load();
}

function setHost(host) {
  if (!host) return;
  menu.value.host = host;
  updateUrl();
  load();
}

function setProbe(probe) {
  if (!probe) return;
  menu.value.probe = probe;
  updateUrl();
  load();
}

function updateUrl() {
  router.replace({
    query: {
      group: menu.value.group || undefined,
      host: menu.value.host || undefined,
      probe: menu.value.probe || undefined,
    },
  });
}

function prev() {
  if (offset.value < limit.value) return;
  offset.value -= limit.value;
  load();
}

function next() {
  offset.value += limit.value;
  load();
}

async function load() {
  try {
    let logsData;

    if (menu.value.probe && menu.value.host) {
      logsData = await api.getProbeLogs(menu.value.host, menu.value.probe, {
        offset: offset.value,
        limit: limit.value,
      });
    } else if (menu.value.host) {
      logsData = await api.getHostLogs(menu.value.host, {
        offset: offset.value,
        limit: limit.value,
      });
    } else if (menu.value.group) {
      logsData = await api.getGroupLogs(menu.value.group, {
        offset: offset.value,
        limit: limit.value,
      });
    } else {
      logsData = await api.getLogs({
        offset: offset.value,
        limit: limit.value,
      });
    }

    logs.value = logsData;

    const indexesData = await api.getLogIndexes();
    indexes.value = indexesData;
  } catch (error) {
    console.error("Error loading logs:", error);
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
  menu.value.group = route.query.group || null;
  menu.value.host = route.query.host || null;
  menu.value.probe = route.query.probe || null;
  offset.value = 0;
  limit.value = 100;
  load();
  startRefresh();
});

watch(
  () => route.query,
  (newQuery) => {
    menu.value.group = newQuery.group || null;
    menu.value.host = newQuery.host || null;
    menu.value.probe = newQuery.probe || null;
    load();
  },
);
</script>
