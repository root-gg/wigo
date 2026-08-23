import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import { STATUS_LEVELS } from "../utils/status.js";

/**
 * Filtrage du dashboard : niveaux de statut et recherche texte.
 *
 * L'état vit dans l'URL (`?levels=...&q=...`) pour qu'une vue filtrée soit
 * partageable et survive à un rechargement. Le dernier choix de niveaux est
 * aussi mémorisé dans le localStorage et sert de valeur par défaut quand l'URL
 * n'en porte pas.
 *
 * Le filtre s'applique toujours à ce que comptent les compteurs de la barre du
 * haut : les hosts sur la vue d'accueil, les probes sur les vues groupe et host.
 */

const STORAGE_KEY = "wigo.levels";
const ALL = "all";
export const PROBLEM_LEVELS = ["WARNING", "CRITICAL", "ERROR"];

function readStoredLevels() {
  try {
    return localStorage.getItem(STORAGE_KEY) || ALL;
  } catch {
    return ALL;
  }
}

function storeLevels(value) {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // Pas de persistance possible, le filtre reste valable pour la session
  }
}

function parseLevels(raw) {
  if (!raw || raw === ALL) {
    return [...STATUS_LEVELS];
  }
  const wanted = String(raw)
    .split(",")
    .map((level) => level.trim().toUpperCase())
    .filter((level) => STATUS_LEVELS.includes(level));

  // Un filtre vide n'afficherait rien : on retombe sur "tout afficher"
  return wanted.length ? wanted : [...STATUS_LEVELS];
}

function serializeLevels(levels) {
  return levels.length === STATUS_LEVELS.length
    ? ALL
    : STATUS_LEVELS.filter((level) => levels.includes(level)).join(",");
}

export function useDashboardFilter() {
  const route = useRoute();
  const router = useRouter();

  const activeLevels = computed(() =>
    parseLevels(route.query.levels ?? readStoredLevels()),
  );

  const search = computed(() => String(route.query.q || "").trim());

  const isFiltered = computed(
    () => activeLevels.value.length !== STATUS_LEVELS.length || !!search.value,
  );

  const problemsOnly = computed(
    () =>
      activeLevels.value.length === PROBLEM_LEVELS.length &&
      PROBLEM_LEVELS.every((level) => activeLevels.value.includes(level)),
  );

  function updateQuery(patch) {
    router.replace({
      path: route.path,
      hash: route.hash,
      query: { ...route.query, ...patch },
    });
  }

  function setLevels(levels) {
    const serialized = serializeLevels(levels);
    storeLevels(serialized);
    updateQuery({ levels: serialized });
  }

  function isLevelActive(level) {
    return activeLevels.value.includes(level);
  }

  function toggleLevel(level) {
    if (!STATUS_LEVELS.includes(level)) return;

    // Depuis "tout afficher", un clic isole le niveau cliqué : c'est le geste
    // attendu quand on clique sur "CRITICAL" pour ne voir que les critiques.
    if (activeLevels.value.length === STATUS_LEVELS.length) {
      setLevels([level]);
      return;
    }

    const next = activeLevels.value.includes(level)
      ? activeLevels.value.filter((item) => item !== level)
      : [...activeLevels.value, level];

    setLevels(next.length ? next : [...STATUS_LEVELS]);
  }

  function toggleProblemsOnly() {
    setLevels(problemsOnly.value ? [...STATUS_LEVELS] : [...PROBLEM_LEVELS]);
  }

  function setSearch(value) {
    const trimmed = String(value || "").trim();
    updateQuery({ q: trimmed || undefined });
  }

  function clearFilters() {
    storeLevels(ALL);
    updateQuery({ levels: undefined, q: undefined });
  }

  /**
   * @param {string} level - Niveau de l'élément
   * @param {...string} haystack - Textes cherchés par la recherche
   */
  function matches(level, ...haystack) {
    if (!activeLevels.value.includes(level)) return false;
    if (!search.value) return true;

    const needle = search.value.toLowerCase();
    return haystack.some(
      (text) => text && String(text).toLowerCase().includes(needle),
    );
  }

  return {
    activeLevels,
    search,
    isFiltered,
    problemsOnly,
    isLevelActive,
    toggleLevel,
    toggleProblemsOnly,
    setSearch,
    clearFilters,
    matches,
  };
}
