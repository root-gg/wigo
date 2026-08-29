<template>
  <nav
    :class="[
      'navbar',
      'navbar-expand',
      'bg-body-tertiary',
      'navbar-block',
      { 'sidebar-collapsed': sidebarCollapsed },
    ]"
  >
    <div class="container-fluid d-flex align-items-center gap-2 flex-wrap">
      <!-- Sous md la barre latérale est un tiroir hors écran, et son propre
           bouton part avec elle. Celui-ci reste. -->
      <button
        type="button"
        class="btn btn-sm btn-outline-secondary d-md-none flex-shrink-0"
        title="Show the hosts and probes"
        aria-label="Show the hosts and probes"
        @click="$emit('toggle-sidebar')"
      >
        <i class="fas fa-bars fa-fw"></i>
      </button>

      <a
        v-if="sidebarCollapsed"
        href="/"
        class="wigo-logo navbar-brand text-body text-decoration-none ms-3 me-2 d-none d-md-inline"
      >
        W I G O
      </a>

      <div v-if="filterable" class="d-flex align-items-center flex-shrink-0">
        <button
          v-for="level in STATUS_LEVELS"
          :key="level"
          type="button"
          :class="[
            'btn',
            'btn-sm',
            'mx-1',
            'status-filter',
            getBtnClass(level),
          ]"
          :aria-pressed="isLevelActive(level)"
          :title="filterTitle(level)"
          @click="toggleLevel(level)"
        >
          <span class="d-none d-xxl-inline">{{ level }}: </span>
          {{ counts[level] || 0 }}
        </button>
      </div>

      <button
        v-if="filterable"
        type="button"
        :class="[
          'btn',
          'btn-sm',
          'flex-shrink-0',
          problemsOnly ? 'btn-secondary' : 'btn-outline-secondary',
        ]"
        :aria-pressed="problemsOnly"
        :title="
          problemsOnly
            ? 'Show every status again'
            : 'Show problems only (hide OK and INFO)'
        "
        @click="toggleProblemsOnly"
      >
        <i
          :class="['fas', 'fa-fw', problemsOnly ? 'fa-eye-slash' : 'fa-eye']"
        ></i>
        <span class="d-none d-xxl-inline">&nbsp;Problems only</span>
      </button>

      <div
        v-if="filterable"
        class="topbar-search flex-grow-1 d-none d-md-block"
      >
        <div class="input-group input-group-sm">
          <span class="input-group-text"><i class="fas fa-search"></i></span>
          <input
            ref="searchInput"
            v-model="localSearch"
            type="search"
            class="form-control"
            placeholder="Search hosts and probes…  (press /)"
            aria-label="Search hosts and probes"
            @input="onSearchInput"
            @keydown.esc="clearSearch"
          />
          <button
            v-if="localSearch"
            class="btn btn-outline-secondary"
            type="button"
            title="Clear the search"
            @click="clearSearch"
          >
            <i class="fas fa-times"></i>
          </button>
        </div>
      </div>

      <!-- Une seule entrée plutôt que sept icônes muettes. Personne ne devine
           qu'une interdiction mène aux sondes désactivées ni qu'une clé mène
           aux jetons : les nommer coûte un clic et fait gagner le survol de
           chacune pour retrouver laquelle est laquelle. -->
      <ul class="navbar-nav ms-auto flex-shrink-0">
        <li class="nav-item dropdown">
          <button
            type="button"
            class="btn btn-sm btn-outline-secondary dropdown-toggle"
            data-bs-toggle="dropdown"
            aria-expanded="false"
            title="Menu"
            aria-label="Menu"
          >
            <i class="fas fa-bars fa-fw"></i>
          </button>

          <ul class="dropdown-menu dropdown-menu-end topbar-menu">
            <li v-for="view in VIEWS" :key="view.to">
              <router-link
                class="dropdown-item"
                :to="view.to"
                :title="view.title"
              >
                <i :class="['fas', 'fa-fw', view.icon]"></i>
                {{ view.label }}
              </router-link>
            </li>

            <li><hr class="dropdown-divider" /></li>
            <li class="dropdown-header">Refresh</li>
            <li v-for="seconds in REFRESH_INTERVALS" :key="seconds">
              <a
                :class="[
                  'dropdown-item',
                  { active: currentInterval === seconds },
                ]"
                href="#"
                :title="`Refresh every ${seconds} seconds`"
                @click.prevent="setInterval(seconds)"
              >
                <i class="fas fa-fw fa-sync"></i> Every {{ seconds }} sec
              </a>
            </li>
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 0 }]"
                href="#"
                title="Disable auto-refresh"
                @click.prevent="setInterval(0)"
              >
                <i class="fas fa-fw fa-stop"></i> Do not refresh
              </a>
            </li>

            <li><hr class="dropdown-divider" /></li>
            <li class="dropdown-header">Theme</li>
            <li v-for="option in THEME_OPTIONS" :key="option.value">
              <a
                :class="[
                  'dropdown-item',
                  { active: preference === option.value },
                ]"
                href="#"
                :title="option.title"
                @click.prevent="setTheme(option.value)"
              >
                <i :class="['fas', 'fa-fw', option.icon]"></i>
                {{ option.label }}
              </a>
            </li>

            <!-- Sur un wigo qui laisse lire sans identifiants, le navigateur
                 n'est jamais challengé : sans ceci l'identifiant existe et
                 reste hors d'atteinte. Une navigation, pas une requête -- le
                 prompt du navigateur n'apparaît de façon fiable que comme ça. -->
            <template v-if="showSignIn">
              <li><hr class="dropdown-divider" /></li>
              <li>
                <a class="dropdown-item" :href="signInUrl">
                  <i class="fas fa-fw fa-right-to-bracket"></i> Sign in
                </a>
              </li>
            </template>

            <template v-if="signedIn">
              <li><hr class="dropdown-divider" /></li>
              <li class="dropdown-header">
                Signed in as <strong>{{ caller.Name }}</strong>
              </li>
              <li>
                <a class="dropdown-item" href="api/logout">
                  <i class="fas fa-fw fa-right-from-bracket"></i> Sign out
                </a>
              </li>
              <li>
                <span class="dropdown-item-text small text-body-secondary">
                  A browser cannot be told to forget a password. Cancelling the
                  prompt it shows next is what clears it.
                </span>
              </li>
            </template>
          </ul>
        </li>
      </ul>
    </div>
  </nav>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import {
  STATUS_LEVELS,
  getBtnLevelClass,
  getBtnOutlineLevelClass,
} from "../../utils/status.js";
import { useDashboardFilter } from "../../composables/useDashboardFilter.js";
import { useTheme } from "../../composables/useTheme.js";
import api from "../../api/client.js";

const REFRESH_INTERVALS = [5, 15, 30, 60, 300];

/** Ce que le menu ouvre. Nommé, parce qu'une icône seule ne l'était pas. */
const VIEWS = [
  {
    to: "/wall",
    icon: "fa-tv",
    label: "Wall",
    title: "A dense grid of every host, for a screen on a wall",
  },
  {
    to: "/quiet",
    icon: "fa-bell-slash",
    label: "Held back",
    title: "Everything whose notifications are held back",
  },
  {
    to: "/disabled",
    icon: "fa-ban",
    label: "Disabled probes",
    title: "Probes disabled across the fleet",
  },
  { to: "/logs", icon: "fa-list", label: "Logs", title: "View logs" },
  {
    to: "/tokens",
    icon: "fa-key",
    label: "API tokens",
    title: "Mint and revoke API tokens",
  },
  {
    to: "/authority",
    icon: "fa-lock",
    label: "Authority",
    title: "Admit and revoke push clients",
  },
];

const THEME_OPTIONS = [
  {
    value: "auto",
    label: "Auto",
    icon: "fa-circle-half-stroke",
    title: "Follow the system theme",
  },
  {
    value: "light",
    label: "Light",
    icon: "fa-sun",
    title: "Always use the light theme",
  },
  {
    value: "dark",
    label: "Dark",
    icon: "fa-moon",
    title: "Always use the dark theme",
  },
];

const props = defineProps({
  counts: {
    type: Object,
    default: () => ({
      OK: 0,
      INFO: 0,
      WARNING: 0,
      CRITICAL: 0,
      ERROR: 0,
    }),
  },
  sidebarCollapsed: {
    type: Boolean,
    default: false,
  },
  currentInterval: {
    type: Number,
    default: 0,
  },
  filterable: {
    type: Boolean,
    default: true,
  },
});

const emit = defineEmits(["refresh-settings", "toggle-sidebar"]);

const {
  search,
  problemsOnly,
  isLevelActive,
  toggleLevel,
  toggleProblemsOnly,
  setSearch,
} = useDashboardFilter();

const { preference, setTheme } = useTheme();

const searchInput = ref(null);
const localSearch = ref(search.value);

// La saisie est locale, l'URL suit après un délai : sans ça chaque frappe
// déclencherait une navigation et un re-rendu de la vue entière.
watch(search, (value) => {
  if (value !== localSearch.value.trim()) {
    localSearch.value = value;
  }
});

let searchTimeout = null;

function onSearchInput() {
  clearTimeout(searchTimeout);
  const value = localSearch.value;
  searchTimeout = setTimeout(() => setSearch(value), 250);
}

function clearSearch() {
  clearTimeout(searchTimeout);
  localSearch.value = "";
  setSearch("");
}

function getBtnClass(level) {
  return isLevelActive(level)
    ? getBtnLevelClass(level)
    : getBtnOutlineLevelClass(level);
}

function filterTitle(level) {
  return isLevelActive(level)
    ? `${level} shown — click to hide`
    : `${level} hidden — click to show`;
}

function setInterval(seconds) {
  emit("refresh-settings", seconds);
}

/** Raccourci "/" pour aller directement dans la recherche */
function handleShortcut(event) {
  if (!props.filterable) return;
  if (event.key !== "/" || event.ctrlKey || event.metaKey || event.altKey) {
    return;
  }
  const target = event.target;
  const tag = target?.tagName?.toLowerCase();
  if (tag === "input" || tag === "textarea" || target?.isContentEditable) {
    return;
  }
  event.preventDefault();
  searchInput.value?.focus();
}

/**
 * Qui on est, et si se connecter est seulement possible.
 *
 * Les vues portent déjà WriteActionsAllowed, mais elles parlent d'un host. La
 * barre du haut doit savoir avant, et pour la session.
 */
const caller = ref({});

/** Identifié par un identifiant, par opposition à laissé entrer sans rien */
const signedIn = computed(
  () =>
    !!caller.value.Name &&
    caller.value.Name !== "anonymous" &&
    caller.value.Name !== "unauthenticated",
);

/**
 * Pas de bouton là où il n'y a pas d'identifiant derrière : sur un wigo sans
 * Login, l'offrir serait une porte sans clé.
 *
 * Le critère est le rôle, pas WriteActionsAllowed : sur un host où les écritures
 * sont fermées, se connecter ne débloque rien, et proposer « sign in to change
 * anything » y serait un mensonge.
 */
const showSignIn = computed(
  () =>
    !!caller.value.CanSignIn &&
    !signedIn.value &&
    caller.value.Role !== "operator",
);

const signInUrl = computed(
  () =>
    `api/login?next=${encodeURIComponent(location.pathname + location.hash)}`,
);

onMounted(async () => {
  window.addEventListener("keydown", handleShortcut);

  try {
    caller.value = await api.getWhoami();
  } catch {
    // Un wigo plus ancien n'a pas cette route : rien à proposer, et rien
    // à signaler non plus
  }
});

onUnmounted(() => {
  window.removeEventListener("keydown", handleShortcut);
  clearTimeout(searchTimeout);
});
</script>

<style scoped>
/* Le menu porte les vues, le rafraîchissement, le thème et la session : sur un
   portable il dépasse, alors il défile plutôt que de sortir de l'écran. */
.topbar-menu {
  max-height: calc(100vh - 4rem);
  overflow-y: auto;
}

/*
 * The search is the only element allowed to shrink, so the bar's minimum width
 * stays the counters + the icons. Without min-width: 0 a flex item refuses to
 * go below its content size and the whole page gains a horizontal scrollbar.
 */
.topbar-search {
  max-width: 26rem;
  min-width: 0;
}

.status-filter {
  transition: opacity 0.15s ease;
}

.status-filter[aria-pressed="false"] {
  opacity: 0.55;
}

.status-filter[aria-pressed="false"]:hover {
  opacity: 1;
}
</style>
