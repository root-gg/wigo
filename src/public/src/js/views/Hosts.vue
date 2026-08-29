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
          role="button"
          tabindex="0"
          :title="groupTitle(group)"
          @click="gotoAnchor(group.Name)"
          @keydown.enter.prevent="gotoAnchor(group.Name)"
          @keydown.space.prevent="gotoAnchor(group.Name)"
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

    <!-- Seulement le refus : ce qui est filtré est dit une fois, dans la barre
         du haut. Une flotte réduite sans rien qui l'explique se lirait comme
         une flotte à laquelle il manque des machines. -->
    <div
      v-if="labelError"
      class="alert alert-warning d-flex align-items-center flex-wrap gap-2 mt-3 mb-0 py-2"
    >
      <i class="fas fa-fw fa-triangle-exclamation"></i>
      <span>{{ labelError }}</span>
      <button
        type="button"
        class="btn btn-sm btn-outline-secondary ms-auto"
        @click="setLabelSelector('')"
      >
        Show every host
      </button>
    </div>

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
import { ref, computed, watch, onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import api from "../api/client.js";
import { getLevel, getStatusRowClass } from "../utils/status.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import StatusBadge from "../components/StatusBadge.vue";
import { useRefresh } from "../composables/useRefresh.js";
import { useLiveEvents } from "../composables/useLiveEvents.js";
import { useDashboardFilter } from "../composables/useDashboardFilter.js";

const router = useRouter();
const groups = ref([]);
const loaded = ref(false);
const counts = ref(emptyCounts());

const { matches, labelSelector, setLabelSelector } = useDashboardFilter();

/** Les hosts que le serveur dit porter le sélecteur, null tant qu'on ignore */
const matchingHosts = ref(null);
const labelError = ref("");

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
        .filter((host) => carriesTheLabels(host.Name))
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

/**
 * Le tri par label est fait par le serveur, pas ici.
 *
 * Le résumé d'un groupe ne porte pas les labels de ses hosts, et les y ajouter
 * grossirait une réponse déjà large pour une question qui se pose rarement.
 * `/api/hosts?labels=` répond exactement ça, et c'est le même code qui décide
 * ici et pour les notifications -- une seule définition de « porte ce label ».
 */
function carriesTheLabels(hostname) {
  if (!labelSelector.value) return true;

  // Tant que la réponse n'est pas là, on ne cache rien : montrer une flotte
  // vide en attendant se lit comme une flotte éteinte.
  if (matchingHosts.value === null) return true;

  return matchingHosts.value.has(hostname);
}

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

/**
 * Qui porte le sélecteur, demandé au serveur.
 *
 * Rechargé avec le reste plutôt qu'une fois pour toutes : un host dont les
 * labels changent, ou qui arrive, doit entrer ou sortir du filtre sans qu'on
 * ait à recharger la page.
 */
async function loadMatchingHosts() {
  if (!labelSelector.value) {
    matchingHosts.value = null;
    labelError.value = "";
    return;
  }

  try {
    const names = await api.getHosts(labelSelector.value);
    matchingHosts.value = new Set(names);
    labelError.value = "";
  } catch (error) {
    // Un sélecteur que le serveur refuse est dit, pas subi : sans ça la page
    // se viderait sans expliquer pourquoi.
    labelError.value =
      error.response?.data || error.message || "That label filter was refused";
    matchingHosts.value = null;
  }
}

watch(labelSelector, loadMatchingHosts);

async function load() {
  try {
    await loadMatchingHosts();

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

// Le flux donne l'immédiateté, le rafraîchissement périodique reste le filet
useLiveEvents(load);

onMounted(() => {
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
