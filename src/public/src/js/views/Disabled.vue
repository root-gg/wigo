<template>
  <AppLayout
    :current-interval="interval"
    :filterable="false"
    title-context="Disabled probes"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Probes disabled across the fleet">
          <i class="fas fa-fw fa-ban"></i><span>&nbsp;Disabled probes</span>
        </a>
      </li>

      <li v-for="host in hostsWithDisabled" :key="host.Name" class="nav-item">
        <a
          class="nav-link px-3 py-1 cursor-pointer"
          :title="`${host.Name} — ${host.DisabledCount} disabled`"
          @click="gotoAnchor(host.Name)"
        >
          <i class="fas fa-fw fa-server"></i
          ><span
            >&nbsp;{{ host.Name }}
            <span class="badge bg-secondary ms-1">{{
              host.DisabledCount
            }}</span>
          </span>
        </a>
      </li>
    </template>

    <!-- Cette page est un filet de sécurité : si une partie du parc n'a pas
         répondu, annoncer un total donnerait une fausse impression de
         complétude. -->
    <div
      v-if="unreachable.length"
      class="alert alert-warning d-flex align-items-start gap-2 mt-4 mb-0 py-2"
    >
      <i class="fas fa-fw fa-triangle-exclamation mt-1"></i>
      <div>
        <div>
          The schedule of {{ unreachable.length }}
          {{ unreachable.length > 1 ? "hosts" : "host" }} could not be read, so
          this list may be incomplete.
        </div>
        <small class="text-body-secondary">
          {{ unreachable.map((host) => host.Name).join(", ") }}
        </small>
      </div>
    </div>

    <div
      v-if="loaded && !hostsWithDisabled.length"
      class="alert alert-success d-flex align-items-center gap-2 mt-4 mb-0 py-2"
    >
      <i class="fas fa-fw fa-check"></i>
      <span>
        No probe is disabled on any host that could be read. Every probe
        installed on them is running.
      </span>
    </div>

    <div
      v-for="host in hostsWithDisabled"
      :key="host.Name"
      :id="host.Name"
      class="jump"
    >
      <StatusCard level="DISABLED">
        <template #title>
          <strong>{{ host.Name }}</strong>
        </template>
        <template #badges>
          <span class="badge text-bg-light">
            {{ host.DisabledCount }} disabled
          </span>
        </template>
        <template #body>
          <!-- Pas de table-responsive ici : son overflow rognerait le menu
               d'ordonnancement, et deux colonnes n'ont rien a faire deborder. -->
          <div>
            <table class="table table-bordered table-hover mb-0">
              <thead>
                <tr>
                  <th>Probe</th>
                  <th class="text-end">Bring it back</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="probe in host.Probes"
                  :key="probe.Name"
                  :class="{ 'table-success': probe.JustEnabled }"
                >
                  <td class="align-middle">
                    <a
                      class="cursor-pointer"
                      :title="`Open ${host.Name}`"
                      @click="gotoHost(host.Name, probe.Name)"
                    >
                      {{ probe.Name }}
                    </a>
                    <span
                      v-if="probe.JustEnabled"
                      class="ms-2 small text-body-secondary"
                    >
                      <i class="fas fa-fw fa-check"></i>
                      enabled, it will run from now on
                    </span>
                  </td>
                  <td>
                    <div class="d-flex justify-content-end">
                      <ProbeScheduleControl
                        :host-name="host.Name"
                        :probe-name="probe.Name"
                        :schedule="probe"
                        :editable="host.WriteActionsAllowed"
                        :read-only-reason="`Read only: set AllowWriteActions in the [Http] section of the configuration file on ${host.Name}`"
                        @changed="
                          (updated) => onChanged(host.Name, probe.Name, updated)
                        "
                      />
                    </div>
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
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import ProbeScheduleControl from "../components/ProbeScheduleControl.vue";
import { useRefresh } from "../composables/useRefresh.js";

const router = useRouter();
const schedules = ref([]);
const unreachable = ref([]);
const loaded = ref(false);

/**
 * Les probes qu'on vient d'activer depuis cette page, par host et par nom.
 *
 * Elles n'ont plus rien à faire dans une liste de probes désactivées, mais les
 * retirer sur-le-champ faisait disparaître la ligne sous le curseur sans rien
 * dire de ce qui venait de se passer. Elles restent affichées, marquées comme
 * activées, jusqu'au rafraîchissement suivant -- ce qui laisse aussi le temps
 * de corriger l'intervalle si on s'est trompé.
 */
const justEnabled = ref(new Map());

function enabledKey(hostName, probeName) {
  return `${hostName}\u0000${probeName}`;
}

// Une probe désactivée est une probe qu'aucun répertoire d'intervalle
// n'ordonnance. La plupart n'ont été coupées par personne : wigo en livre une
// trentaine et le packaging en active la moitié. Le résultat est le même, une
// vérification qui n'a pas lieu, et c'est ce que cette page recense.
const hostsWithDisabled = computed(() =>
  schedules.value
    .map((schedule) => {
      const disabled = (schedule.Probes || []).filter(
        (location) => !location.Enabled,
      );
      const stillListed = new Set(disabled.map((location) => location.Name));

      const enabled = [...justEnabled.value.entries()]
        .filter(
          ([key, location]) =>
            key.startsWith(`${schedule.Hostname}\u0000`) &&
            !stillListed.has(location.Name),
        )
        .map(([, location]) => ({ ...location, JustEnabled: true }));

      return {
        Name: schedule.Hostname,
        WriteActionsAllowed: !!schedule.WriteActionsAllowed,
        DisabledCount: disabled.length,
        Probes: [...disabled, ...enabled].sort((a, b) =>
          a.Name.localeCompare(b.Name),
        ),
      };
    })
    .filter((host) => host.Probes.length > 0)
    .sort((a, b) => a.Name.localeCompare(b.Name)),
);

/**
 * L'API répond l'ordonnancement complet du host visé, donc l'état de la probe
 * est celui qu'elle a réellement pris, pas celui qu'on a demandé.
 */
function onChanged(hostName, probeName, updated) {
  const location = (updated?.Probes || []).find(
    (probe) => probe.Name === probeName,
  );

  if (location && location.Enabled) {
    justEnabled.value.set(enabledKey(hostName, probeName), location);
  } else {
    justEnabled.value.delete(enabledKey(hostName, probeName));
  }
  justEnabled.value = new Map(justEnabled.value);

  load();
}

function gotoAnchor(anchor) {
  const element = document.getElementById(anchor);
  if (element) {
    element.scrollIntoView({ behavior: "smooth" });
  }
}

function gotoHost(hostName, probeName) {
  router.push({
    path: "/host",
    query: { name: hostName },
    hash: `#${probeName}`,
  });
}

async function load() {
  try {
    const hostNames = await api.getHosts();

    // Le master relaie une demande par host : rien n'est agrégé côté serveur,
    // ce qui évite qu'un seul remote lent bloque toute la page.
    const answers = await Promise.all(
      hostNames.map((hostName) =>
        api
          .getHostProbesSchedule(hostName)
          .then((schedule) => ({ hostName, schedule }))
          .catch(() => ({ hostName, schedule: null })),
      ),
    );

    schedules.value = answers
      .filter((answer) => answer.schedule)
      .map((answer) => answer.schedule);

    unreachable.value = answers
      .filter((answer) => !answer.schedule)
      .map((answer) => ({ Name: answer.hostName }));

    loaded.value = true;
  } catch (error) {
    console.error("Error loading the disabled probes:", error);
  }
}

// Le rafraîchissement périodique est aussi ce qui fait retomber les lignes des
// probes qu'on vient d'activer : elles ont eu le temps d'être vues.
function refresh() {
  justEnabled.value = new Map();
  return load();
}

const { startRefresh, stopRefresh, setRefreshInterval, interval } = useRefresh(
  refresh,
  60,
);

function handleRefreshSettings(seconds) {
  setRefreshInterval(seconds);
}

onMounted(() => {
  refresh();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
