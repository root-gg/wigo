<template>
  <div class="wall" :class="{ 'wall--dense': tiles.length > 60 }">
    <!-- Une page figée et un parc en bonne santé se ressemblent exactement.
         L'heure du dernier rafraîchissement est donc la première chose de
         cette vue : sans elle, un écran mort se lit comme « tout va bien ». -->
    <header class="wall__header">
      <div class="d-flex align-items-baseline gap-3 flex-wrap">
        <span class="wall__brand">W I G O</span>
        <span
          v-for="level in STATUS_LEVELS"
          :key="level"
          v-show="counts[level]"
          :class="['wall__count', getBadgeLevelClass(level)]"
        >
          {{ counts[level] }} {{ level }}
        </span>
        <span v-if="!problems" class="wall__count badge bg-success">
          all {{ tiles.length }} hosts fine
        </span>
      </div>

      <div class="d-flex align-items-center gap-3">
        <span :class="['wall__updated', stale ? 'wall__updated--stale' : '']">
          <i
            :class="[
              'fas',
              'fa-fw',
              stale ? 'fa-triangle-exclamation' : 'fa-clock',
            ]"
          ></i>
          {{ updatedLabel }}
        </span>
        <button
          type="button"
          class="btn btn-sm btn-outline-light"
          :title="fullscreen ? 'Leave fullscreen' : 'Fullscreen'"
          @click="toggleFullscreen"
        >
          <i
            :class="['fas', 'fa-fw', fullscreen ? 'fa-compress' : 'fa-expand']"
          ></i>
        </button>
        <router-link
          class="btn btn-sm btn-outline-light"
          to="/"
          title="Leave the wall"
        >
          <i class="fas fa-fw fa-xmark"></i>
        </router-link>
      </div>
    </header>

    <p v-if="error" class="wall__empty">{{ error }}</p>
    <p v-else-if="loaded && !tiles.length" class="wall__empty">
      No hosts to show.
    </p>

    <!-- Le pire en premier : sur un mur, le regard part en haut à gauche, et
         c'est là que doit se trouver ce qui est cassé. -->
    <div v-else class="wall__grid">
      <router-link
        v-for="tile in tiles"
        :key="tile.name"
        :to="hostRoute(tile.name)"
        :class="['wall__tile', getBadgeLevelClass(tile.level)]"
        :title="tile.message"
      >
        <span class="wall__group">{{ tile.group }}</span>
        <span class="wall__host">{{ tile.short }}</span>
        <span class="wall__note">{{ tile.note }}</span>
      </router-link>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import api from "../api/client.js";
import { useRefresh } from "../composables/useRefresh.js";
import { useLiveEvents } from "../composables/useLiveEvents.js";
import {
  STATUS_LEVELS,
  getLevel,
  getBadgeLevelClass,
} from "../utils/status.js";

/** Au-delà, un écran non rafraîchi ne raconte plus le présent */
const STALE_AFTER = 180;

const tiles = ref([]);
const counts = ref({});
const loaded = ref(false);
const error = ref("");
const updatedAt = ref(0);
const now = ref(Math.floor(Date.now() / 1000));
const fullscreen = ref(false);

const problems = computed(() =>
  tiles.value.some((tile) => tile.level !== "OK"),
);

const stale = computed(
  () => updatedAt.value > 0 && now.value - updatedAt.value > STALE_AFTER,
);

const updatedLabel = computed(() => {
  if (!updatedAt.value) return "never updated";

  const seconds = now.value - updatedAt.value;
  if (seconds < 60) return `updated ${seconds}s ago`;

  return `updated ${Math.floor(seconds / 60)}m ago`;
});

function hostRoute(name) {
  return { path: "/host", query: { name } };
}

/**
 * Le pire d'abord, puis par nom.
 *
 * Trié sur le statut numérique et non sur le niveau : entre deux critiques,
 * 400 est pire que 300, et l'ordre alphabétique des niveaux ne le dit pas.
 */
function worstFirst(a, b) {
  if (a.status !== b.status) return b.status - a.status;

  return a.name.localeCompare(b.name);
}

async function load() {
  try {
    const groupNames = await api.getGroups();

    const groups = await Promise.all(
      groupNames.map((name) => api.getGroup(name).catch(() => null)),
    );

    const nextTiles = [];
    const nextCounts = {};

    for (const group of groups) {
      if (!group) continue;

      for (const host of group.Hosts) {
        const level = getLevel(host.Status);
        nextCounts[level] = (nextCounts[level] || 0) + 1;

        const failing = (host.Probes || []).filter(
          (probe) => probe.Status > 100,
        );
        const worst = failing.sort((a, b) => b.Status - a.Status)[0];

        nextTiles.push({
          name: host.Name,
          // Le domaine est le même pour tout le monde et mange la place :
          // ce qui distingue une machine d'une autre est devant.
          short: host.Name.split(".")[0],
          group: group.Name,
          status: host.Status,
          level,
          note: describe(host, failing, worst),
          message: worst ? `${worst.Name} : ${worst.Message}` : host.Message,
        });
      }
    }

    tiles.value = nextTiles.sort(worstFirst);
    counts.value = nextCounts;
    updatedAt.value = Math.floor(Date.now() / 1000);
    loaded.value = true;
    error.value = "";
  } catch (requestError) {
    // L'affichage précédent reste : sur un mur, une page vide ne dit pas
    // « la requête a échoué », elle dit « il n'y a rien », ce qui est faux.
    error.value = tiles.value.length
      ? ""
      : requestError.message || "Nothing could be loaded";
  }
}

function describe(host, failing, worst) {
  if (!host.IsAlive) return "not answering";
  if (!failing.length) return "";
  if (failing.length === 1) return worst.Name;

  return `${worst.Name} +${failing.length - 1}`;
}

function toggleFullscreen() {
  if (document.fullscreenElement) {
    document.exitFullscreen();
    return;
  }

  document.documentElement.requestFullscreen().catch(() => {
    // Refusé faute de geste utilisateur reconnu : rien à dire de plus
  });
}

function trackFullscreen() {
  fullscreen.value = !!document.fullscreenElement;
}

const { startRefresh, stopRefresh } = useRefresh(load, 60);

useLiveEvents(load);

// L'âge affiché doit vieillir tout seul, sinon « updated 3s ago » reste à
// l'écran une heure après que le flux est mort.
let ticker = null;

onMounted(() => {
  load();
  startRefresh();
  document.addEventListener("fullscreenchange", trackFullscreen);
  ticker = setInterval(() => {
    now.value = Math.floor(Date.now() / 1000);
  }, 1000);
});

onUnmounted(() => {
  stopRefresh();
  document.removeEventListener("fullscreenchange", trackFullscreen);
  if (ticker) clearInterval(ticker);
});
</script>

<style scoped>
.wall {
  min-height: 100vh;
  padding: 0.75rem;
  background: #1a1d21;
  color: #fff;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.wall__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  flex-wrap: wrap;
}

.wall__brand {
  font-weight: 700;
  letter-spacing: 0.3em;
}

.wall__count {
  font-size: 1rem;
  padding: 0.35em 0.7em;
}

.wall__updated {
  font-variant-numeric: tabular-nums;
  opacity: 0.7;
}

/* Un écran qui ne se rafraîchit plus tout en paraissant à jour est pire que
   pas d'écran du tout. */
.wall__updated--stale {
  opacity: 1;
  color: var(--bs-warning);
  font-weight: 600;
}

/* Les tuiles remplissent la hauteur : un mur qui laisse la moitié de l'écran
   vide se lit de moins loin, et se lire de loin est tout ce qu'on lui demande. */
.wall__grid {
  flex: 1;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
  grid-auto-rows: minmax(5.5rem, 1fr);
  gap: 0.5rem;
}

.wall--dense .wall__grid {
  grid-template-columns: repeat(auto-fill, minmax(7.5rem, 1fr));
  grid-auto-rows: minmax(4rem, 1fr);
}

.wall__tile {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 0.15rem;
  padding: 0.6rem 0.7rem;
  border-radius: 0.4rem;
  overflow: hidden;
  cursor: pointer;
  text-decoration: none;
}

/* Le badge apporte le fond et la couleur de texte lisible dessus ; le reste
   de sa mise en forme ne convient pas à une tuile. */
.wall__tile {
  font-weight: 400;
  white-space: normal;
  text-align: left;
}

.wall__group {
  font-size: 0.7rem;
  opacity: 0.75;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.wall__host {
  font-size: 1.35rem;
  font-weight: 600;
  line-height: 1.15;
  word-break: break-word;
}

.wall--dense .wall__host {
  font-size: 1rem;
}

.wall__note {
  font-size: 0.8rem;
  opacity: 0.85;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wall__empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0.7;
}
</style>
