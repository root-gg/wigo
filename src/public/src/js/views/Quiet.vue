<template>
  <AppLayout
    :current-interval="interval"
    :filterable="false"
    title-context="Held back"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a
          class="nav-link px-3 py-1"
          title="Everything whose notifications are currently held back"
        >
          <i class="fas fa-fw fa-bell-slash"></i><span>&nbsp;Held back</span>
        </a>
      </li>
    </template>

    <div
      v-if="loaded && !suppressions.length && !flapping.length"
      class="alert alert-success d-flex align-items-center gap-2 mt-4 mb-0 py-2"
    >
      <i class="fas fa-fw fa-check"></i>
      <span>
        Nothing is held back. Every problem this wigo sees would be notified
        about.
      </span>
    </div>

    <!-- Une décision de quelqu'un. La lever se fait ici, sans avoir à retrouver
         la carte où elle a été prise -- un silence posé sur un groupe n'en a
         d'ailleurs aucune. -->
    <StatusCard v-if="suppressions.length" level="DISABLED">
      <template #title><strong>Acknowledged and silenced</strong></template>
      <template #badges>
        <span class="badge text-bg-light">{{ suppressions.length }}</span>
      </template>
      <template #body>
        <div>
          <table class="table table-bordered table-hover mb-0">
            <thead>
              <tr>
                <th>What</th>
                <th>Why it is quiet</th>
                <th class="text-end">Notify me again</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="one in suppressions" :key="keyOf(one)">
                <td class="align-middle">
                  <span
                    :class="[
                      'badge',
                      'me-2',
                      one.Kind === 'ack' ? 'text-bg-info' : 'text-bg-secondary',
                    ]"
                  >
                    <i
                      :class="[
                        'fas',
                        'fa-fw',
                        one.Kind === 'ack' ? 'fa-user-check' : 'fa-bell-slash',
                      ]"
                    ></i>
                    {{ one.Kind }}
                  </span>

                  <a
                    v-if="one.Scope === 'host'"
                    class="cursor-pointer"
                    :title="`Open ${one.Target}`"
                    @click="gotoHost(one.Target, one.Probe)"
                  >
                    {{ describeTarget(one) }}
                  </a>
                  <span v-else>{{ describeTarget(one) }}</span>
                </td>

                <td class="align-middle small">
                  <div>{{ describeSuppression(one) }}</div>
                </td>

                <td class="align-middle text-end">
                  <button
                    v-if="writeAllowed"
                    class="btn btn-sm btn-outline-primary"
                    type="button"
                    @click="lift(one)"
                  >
                    <i class="fas fa-fw fa-bell"></i>
                    {{ one.Kind === "ack" ? "Un-acknowledge" : "Un-silence" }}
                  </button>
                  <span
                    v-else
                    class="text-body-secondary"
                    :title="readOnlyReason"
                  >
                    <i class="fas fa-fw fa-lock"></i>
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </StatusCard>

    <!-- Personne n'a décidé ça, wigo l'a fait. L'effet est le même, des
         notifications retenues, donc ça se lit au même endroit. -->
    <StatusCard v-if="flapping.length" level="WARNING">
      <template #title><strong>Flapping</strong></template>
      <template #badges>
        <span class="badge text-bg-light">{{ flapping.length }}</span>
      </template>
      <template #body>
        <p class="small text-body-secondary">
          These probes changed status often enough to be called out once and
          then left alone. Nobody decided it and there is nothing to lift: they
          go back to notifying as soon as they settle.
        </p>
        <ul class="mb-0">
          <li v-for="name in flapping" :key="name">
            <a
              class="cursor-pointer"
              @click="gotoHost(name.split('/')[0], name.split('/')[1])"
            >
              {{ name }}
            </a>
          </li>
        </ul>
      </template>
    </StatusCard>

    <p v-if="error" class="text-danger small mt-3" role="alert">
      <i class="fas fa-fw fa-triangle-exclamation"></i> {{ error }}
    </p>
  </AppLayout>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import api from "../api/client.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import { useRefresh } from "../composables/useRefresh.js";
import { useLiveEvents } from "../composables/useLiveEvents.js";
import { describeSuppression } from "../utils/suppression.js";

const router = useRouter();
const suppressions = ref([]);
const flapping = ref([]);
const writeAllowed = ref(false);
const loaded = ref(false);
const error = ref("");

const readOnlyReason =
  "Read only: an operator credential is needed to lift a suppression";

function keyOf(one) {
  return `${one.Scope} ${one.Target} ${one.Probe || ""}`;
}

function describeTarget(one) {
  if (one.Scope === "group") return `group ${one.Target}`;
  if (one.Probe) return `${one.Target} / ${one.Probe}`;

  return one.Target;
}

function gotoHost(hostName, probeName) {
  router.push({
    path: "/host",
    query: { name: hostName },
    hash: probeName ? `#${probeName}` : undefined,
  });
}

async function load() {
  try {
    const answer = await api.getSuppressions();
    suppressions.value = answer.Suppressions || [];
    flapping.value = answer.Flapping || [];
    writeAllowed.value = !!answer.WriteActionsAllowed;
    loaded.value = true;
  } catch (requestError) {
    console.error("Error loading what is held back:", requestError);
  }
}

async function lift(one) {
  error.value = "";
  try {
    const answer =
      one.Scope === "group"
        ? await api.unsuppressGroup(one.Target)
        : await api.unsuppressHost(one.Target, one.Probe);

    suppressions.value = answer.Suppressions || [];
    flapping.value = answer.Flapping || [];
  } catch (requestError) {
    error.value =
      requestError.response?.data ||
      requestError.message ||
      "The request failed";
  }
}

const { startRefresh, stopRefresh, setRefreshInterval, interval } = useRefresh(
  load,
  60,
);

function handleRefreshSettings(seconds) {
  setRefreshInterval(seconds);
}

useLiveEvents(load);

onMounted(() => {
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
