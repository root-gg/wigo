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
    <div class="container-fluid d-flex align-items-center gap-2">
      <a
        v-if="sidebarCollapsed"
        href="/"
        class="wigo-logo navbar-brand text-body text-decoration-none ms-3 me-2"
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

      <ul class="navbar-nav ms-auto flex-shrink-0">
        <li class="nav-item dropdown">
          <a
            class="nav-link dropdown-toggle"
            href="#"
            role="button"
            data-bs-toggle="dropdown"
            aria-expanded="false"
            title="Theme"
          >
            <i :class="['fas', 'fa-fw', themeIcon]"></i>
          </a>
          <ul class="dropdown-menu dropdown-menu-end">
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
          </ul>
        </li>
        <li class="nav-item dropdown">
          <a
            class="nav-link dropdown-toggle"
            href="#"
            role="button"
            data-bs-toggle="dropdown"
            aria-expanded="false"
            title="Refresh settings"
          >
            <i class="fas fa-sync fa-fw"></i>
          </a>
          <ul class="dropdown-menu dropdown-menu-end">
            <li v-for="seconds in REFRESH_INTERVALS" :key="seconds">
              <a
                :class="[
                  'dropdown-item',
                  { active: currentInterval === seconds },
                ]"
                href="#"
                @click.prevent="setInterval(seconds)"
                :title="`Refresh every ${seconds} seconds`"
              >
                <i class="fas fa-gear fa-fw"></i> {{ seconds }} sec
              </a>
            </li>
            <li><hr class="dropdown-divider" /></li>
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 0 }]"
                href="#"
                @click.prevent="setInterval(0)"
                title="Disable auto-refresh"
              >
                <i class="fas fa-stop fa-fw"></i> Disable
              </a>
            </li>
          </ul>
        </li>
        <li class="nav-item">
          <router-link
            class="nav-link"
            to="/quiet"
            title="Everything whose notifications are held back"
          >
            <i class="fas fa-bell-slash fa-fw"></i>
          </router-link>
        </li>
        <li class="nav-item">
          <router-link class="nav-link" to="/tokens" title="API tokens">
            <i class="fas fa-key fa-fw"></i>
          </router-link>
        </li>
        <li class="nav-item">
          <router-link
            class="nav-link"
            to="/disabled"
            title="Probes disabled across the fleet"
          >
            <i class="fas fa-ban fa-fw"></i>
          </router-link>
        </li>
        <li class="nav-item">
          <router-link class="nav-link" to="/logs" title="View logs">
            <i class="fas fa-list fa-fw"></i>
          </router-link>
        </li>
        <li class="nav-item">
          <router-link
            class="nav-link"
            to="/authority"
            title="Authority settings"
          >
            <i class="fas fa-lock fa-fw"></i>
          </router-link>
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

const REFRESH_INTERVALS = [5, 15, 30, 60, 300];

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

const emit = defineEmits(["refresh-settings"]);

const {
  search,
  problemsOnly,
  isLevelActive,
  toggleLevel,
  toggleProblemsOnly,
  setSearch,
} = useDashboardFilter();

const { preference, resolvedTheme, setTheme } = useTheme();

const themeIcon = computed(() => {
  if (preference.value === "auto") return "fa-circle-half-stroke";
  return resolvedTheme.value === "dark" ? "fa-moon" : "fa-sun";
});

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

onMounted(() => {
  window.addEventListener("keydown", handleShortcut);
});

onUnmounted(() => {
  window.removeEventListener("keydown", handleShortcut);
  clearTimeout(searchTimeout);
});
</script>

<style scoped>
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
