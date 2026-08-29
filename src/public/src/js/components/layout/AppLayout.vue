<template>
  <div class="d-flex flex-nowrap" style="min-height: 100vh">
    <!-- Sur mobile le tiroir couvre le contenu : choisir une entrée doit le
         refermer, sinon on atterrit derrière lui. -->
    <Sidebar
      :collapsed="sidebarCollapsed"
      @toggle="toggleSidebar"
      @click="closeDrawerOnNarrowScreens"
    >
      <slot name="sidebar"></slot>
    </Sidebar>

    <!-- Un voile, pour que toucher à côté referme -- et pour que le contenu
         derrière se lise comme inactif plutôt que comme cliquable. -->
    <div
      v-if="!sidebarCollapsed"
      class="sidebar-backdrop d-md-none"
      @click="toggleSidebar"
    ></div>

    <!-- min-width-0 : sans ça le panneau refuse de passer sous la largeur de
         son contenu et la page entière déborde horizontalement -->
    <div class="flex-grow-1 min-width-0 bg-body-secondary">
      <TopBar
        :counts="counts"
        :filterable="filterable"
        :sidebar-collapsed="sidebarCollapsed"
        :current-interval="currentInterval"
        @refresh-settings="handleRefreshSettings"
        @toggle-sidebar="toggleSidebar"
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

const { activeLevels, search, labelSelector, isFiltered, clearFilters } =
  useDashboardFilter();

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
  // Dit ici plutôt que dans une seconde bannière : deux barres pour une même
  // chose, dont une vide quand seul le label filtre, se lit comme un bug.
  if (labelSelector.value) {
    parts.push(`with ${labelSelector.value}`);
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

/**
 * En dessous de md, la sidebar est un tiroir posé sur le contenu.
 *
 * On interroge la même requête que la feuille de style plutôt que de comparer
 * innerWidth à 768 : un `<th>` collant dans un cadre défilant gonfle
 * innerWidth -- 881 sur un écran de 390 -- et la comparaison disait alors
 * « grand écran » sur un téléphone. Le tiroir ne se refermait jamais.
 */
const DRAWER_QUERY = "(max-width: 767.98px)";

function closeDrawerOnNarrowScreens() {
  if (sidebarCollapsed.value) return;
  if (!window.matchMedia(DRAWER_QUERY).matches) return;

  toggleSidebar();
}

function handleRefreshSettings(seconds) {
  emit("refresh-settings", seconds);
}
</script>

<style scoped>
.sidebar-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1040;
  background: rgba(0, 0, 0, 0.5);
}
</style>
