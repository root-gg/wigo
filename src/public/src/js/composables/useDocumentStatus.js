import { unref, watchEffect } from "vue";
import { STATUS_LEVELS } from "../utils/status.js";

/**
 * Reflète l'état de la supervision dans le titre de l'onglet et la favicon,
 * pour que le dashboard reste lisible depuis un onglet en arrière-plan.
 */

const BASE_TITLE = "W I G O";

/** Couleurs Bootstrap correspondant aux classes bg-* utilisées par l'UI. */
const LEVEL_COLORS = {
  OK: "#198754",
  INFO: "#0d6efd",
  WARNING: "#ffc107",
  CRITICAL: "#dc3545",
  ERROR: "#212529",
};

/** Du plus grave au moins grave : le premier niveau non vide gagne. */
const SEVERITY_ORDER = ["ERROR", "CRITICAL", "WARNING", "INFO"];

function faviconDataUri(color) {
  const svg =
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">' +
    `<circle cx="8" cy="8" r="7" fill="${color}"/>` +
    "</svg>";
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

function setFavicon(href) {
  let link = document.querySelector('link[rel~="icon"]');
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    document.head.appendChild(link);
  }
  if (link.getAttribute("href") !== href) {
    link.setAttribute("href", href);
  }
}

/**
 * @param {Object|import("vue").Ref} counts - Compteurs par niveau
 * @param {string|import("vue").Ref} context - Contexte affiché (nom du host, du groupe...)
 */
export function useDocumentStatus(counts, context = "") {
  watchEffect(() => {
    const values = unref(counts) || {};
    const label = unref(context);

    const worst = SEVERITY_ORDER.find((level) => values[level] > 0);
    const parts = [];

    if (worst) {
      parts.push(`(${values[worst]}) ${worst}`);
    }
    if (label) {
      parts.push(label);
    }
    parts.push(BASE_TITLE);

    document.title = parts.join(" — ");

    const hasAnyCount = STATUS_LEVELS.some((level) => values[level] > 0);
    if (!hasAnyCount) {
      setFavicon("/favicon.ico");
      return;
    }
    setFavicon(faviconDataUri(LEVEL_COLORS[worst || "OK"]));
  });
}
