<template>
  <AppLayout
    :current-interval="interval"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Authority management">
          <i class="fas fa-fw fa-key"></i
          ><span>&nbsp;Authority management</span>
        </a>
      </li>

      <li class="nav-item">
        <a class="nav-link px-3 py-1" title="Allow all waiting clients">
          <button
            type="button"
            class="btn btn-success btn-sm w-100"
            @click="allowAll"
          >
            <i class="fas fa-check-circle me-1"></i>
            Allow all waiting clients
          </button>
        </a>
      </li>
      <li class="nav-item">
        <a class="nav-link px-3 py-1" title="Revoke all clients">
          <button
            type="button"
            class="btn btn-danger btn-sm w-100"
            @click="revokeAll"
          >
            <i class="fas fa-times-circle me-1"></i>
            Revoke all clients
          </button>
        </a>
      </li>
    </template>

    <div v-if="waiting.length" class="card my-4">
      <div class="card-header text-white bg-warning">
        <span class="fw-bold">
          <i class="fas fa-clock me-2"></i>
          Clients waiting for approval
        </span>
      </div>
      <div class="card-body">
        <div class="table-responsive">
          <table class="table table-bordered table-hover">
            <thead>
              <tr>
                <th>Hostname</th>
                <th>Uuid</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="host in sortedWaiting" :key="host.uuid">
                <td>{{ host.hostname }}</td>
                <td>
                  <code class="small">{{ host.uuid }}</code>
                </td>
                <td>
                  <div class="btn-group">
                    <button
                      type="button"
                      class="btn btn-danger btn-sm"
                      @click="revoke(host)"
                      title="Revoke"
                    >
                      <i class="fas fa-times"></i>
                    </button>
                    <button
                      type="button"
                      class="btn btn-success btn-sm"
                      @click="allow(host)"
                      title="Allow"
                    >
                      <i class="fas fa-check"></i>
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div v-if="allowed.length" class="card my-4">
      <div class="card-header text-white bg-success">
        <span class="fw-bold">
          <i class="fas fa-check-circle me-2"></i>
          Allowed clients
        </span>
      </div>
      <div class="card-body">
        <div class="table-responsive">
          <table class="table table-bordered table-hover">
            <thead>
              <tr>
                <th>Hostname</th>
                <th>Uuid</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="host in sortedAllowed"
                :key="host.uuid"
                class="cursor-pointer"
                @click="gotoHost(host.hostname)"
              >
                <td>{{ host.hostname }}</td>
                <td>
                  <code class="small">{{ host.uuid }}</code>
                </td>
                <td>
                  <button
                    type="button"
                    class="btn btn-danger btn-sm"
                    @click.stop="revoke(host)"
                    title="Revoke"
                  >
                    <i class="fas fa-times"></i>
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div v-if="!waiting.length && !allowed.length" class="card my-4">
      <div class="card-body text-center text-muted py-5">
        <i class="fas fa-inbox fa-3x mb-3"></i>
        <p>No clients waiting for approval or allowed.</p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import api from "../api/client.js";
import AppLayout from "../components/layout/AppLayout.vue";
import { useRefresh } from "../composables/useRefresh.js";

const router = useRouter();
const waiting = ref([]);
const allowed = ref([]);

const sortedWaiting = computed(() => {
  return [...waiting.value].sort((a, b) =>
    a.hostname.localeCompare(b.hostname),
  );
});

const sortedAllowed = computed(() => {
  return [...allowed.value].sort((a, b) =>
    a.hostname.localeCompare(b.hostname),
  );
});

function gotoHost(hostname) {
  router.push({ path: "/host", query: { name: hostname } });
}

async function load() {
  try {
    const hosts = await api.getAuthorityHosts();

    waiting.value = [];
    allowed.value = [];

    if (hosts.waiting) {
      for (const [uuid, hostname] of Object.entries(hosts.waiting)) {
        waiting.value.push({ uuid, hostname });
      }
    }

    if (hosts.allowed) {
      for (const [uuid, hostname] of Object.entries(hosts.allowed)) {
        allowed.value.push({ uuid, hostname });
      }
    }
  } catch (error) {
    console.error("Error loading authority:", error);
  }
}

async function allow(host) {
  try {
    await api.allowHost(host.uuid);
    await load();
  } catch (error) {
    console.error("Error allowing host:", error);
  }
}

async function allowAll() {
  try {
    const promises = waiting.value.map((host) => api.allowHost(host.uuid));
    await Promise.all(promises);
    await load();
  } catch (error) {
    console.error("Error allowing all hosts:", error);
  }
}

async function revoke(host) {
  try {
    await api.revokeHost(host.uuid);
    await load();
  } catch (error) {
    console.error("Error revoking host:", error);
  }
}

async function revokeAll() {
  try {
    const promises = allowed.value.map((host) => api.revokeHost(host.uuid));
    await Promise.all(promises);
    await load();
  } catch (error) {
    console.error("Error revoking all hosts:", error);
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
