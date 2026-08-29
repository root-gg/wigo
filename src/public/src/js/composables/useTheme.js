import { computed, ref, watch } from "vue";

/**
 * Thème de l'interface : "auto" suit les préférences du système, "light" et
 * "dark" les forcent. L'état est partagé par tous les composants (state au
 * niveau du module) et persisté dans le localStorage.
 */

const STORAGE_KEY = "wigo.theme";
export const THEMES = ["auto", "light", "dark"];

function readStoredPreference() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return THEMES.includes(stored) ? stored : "auto";
  } catch {
    // localStorage peut être inaccessible (navigation privée, cookies bloqués)
    return "auto";
  }
}

const preference = ref(readStoredPreference());
const systemPrefersDark = ref(false);

const resolvedTheme = computed(() => {
  if (preference.value === "auto") {
    return systemPrefersDark.value ? "dark" : "light";
  }
  return preference.value;
});

function applyTheme() {
  document.documentElement.dataset.bsTheme = resolvedTheme.value;
}

/** À appeler une seule fois, au démarrage de l'application. */
export function initTheme() {
  const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  systemPrefersDark.value = mediaQuery.matches;
  mediaQuery.addEventListener("change", (event) => {
    systemPrefersDark.value = event.matches;
  });

  applyTheme();
  watch(resolvedTheme, applyTheme);
}

export function useTheme() {
  function setTheme(value) {
    if (!THEMES.includes(value)) return;
    preference.value = value;
    try {
      localStorage.setItem(STORAGE_KEY, value);
    } catch {
      // Pas de persistance possible, le thème reste valable pour la session
    }
  }

  return { preference, resolvedTheme, setTheme };
}
