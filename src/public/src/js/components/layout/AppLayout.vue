<template>
  <div class="d-flex flex-nowrap" style="min-height: 100vh">
    <Sidebar :collapsed="sidebarCollapsed" @toggle="toggleSidebar">
      <slot name="sidebar"></slot>
    </Sidebar>

    <!-- min-width-0 : sans ça le panneau refuse de passer sous la largeur de
         son contenu et la page entière déborde horizontalement -->
    <div class="flex-grow-1 min-width-0 bg-body-secondary">
      <TopBar
        :counts="counts"
        :filterable="filterable"
        :sidebar-collapsed="sidebarCollapsed"
        :current-interval="currentInterval"
        @refresh-settings="handleRefreshSettings"
      />

      <main
        class="content-wrapper"
        :class="{ 'sidebar-collapsed': sidebarCollapsed }"
      >
        <div class="container-fluid">
          <div
            v-if="filterable && isFiltered"
            class="alert alert-secondary d-flex align-items-center gap-2 mt-4 mb-0 py-2"
          >
            <i class="fas fa-filter"></i>
            <span>{{ filterSummary }}</span>
            <button
              type="button"
              class="btn btn-sm btn-outline-secondary ms-auto"
              @click="clearFilters"
            >
              Clear filters
            </button>
          </div>
          <slot></slot>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from "vue";
import Sidebar from "./Sidebar.vue";
import TopBar from "./TopBar.vue";
import { useDashboardFilter } from "../../composables/useDashboardFilter.js";
import { useDocumentStatus } from "../../composables/useDocumentStatus.js";
import { STATUS_LEVELS } from "../../utils/status.js";

const SIDEBAR_STORAGE_KEY = "wigo.sidebarCollapsed";
/** Sidebar repliée par défaut sur les petits écrans (< 992px) */
const SIDEBAR_WIDE_BREAKPOINT = 992;

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
  currentInterval: {
    type: Number,
    default: 0,
  },
  /** Contexte affiché dans le titre de l'onglet (nom du host, du groupe...) */
  titleContext: {
    type: String,
    default: "",
  },
  /** Les vues qui n'affichent ni host ni probe masquent les filtres */
  filterable: {
    type: Boolean,
    default: true,
  },
});

const emit = defineEmits(["refresh-settings"]);

const { activeLevels, search, isFiltered, clearFilters } = useDashboardFilter();

useDocumentStatus(
  computed(() => props.counts),
  computed(() => props.titleContext),
);

const filterSummary = computed(() => {
  const parts = [];
  if (activeLevels.value.length !== STATUS_LEVELS.length) {
    parts.push(`showing ${activeLevels.value.join(", ")}`);
  }
  if (search.value) {
    parts.push(`matching “${search.value}”`);
  }
  return `Filtered: ${parts.join(", ")}`;
});

function readStoredSidebarState() {
  try {
    const stored = localStorage.getItem(SIDEBAR_STORAGE_KEY);
    if (stored === "true") return true;
    if (stored === "false") return false;
  } catch {
    // Pas de persistance disponible, on retombe sur la largeur de l'écran
  }
  return window.innerWidth < SIDEBAR_WIDE_BREAKPOINT;
}

const sidebarCollapsed = ref(readStoredSidebarState());

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value;
  try {
    localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarCollapsed.value));
  } catch {
    // Le repli reste valable pour la session
  }
}

function handleRefreshSettings(seconds) {
  emit("refresh-settings", seconds);
}
</script>
