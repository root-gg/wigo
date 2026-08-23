/**
 * Utilitaires pour la gestion des niveaux de statut
 */

export const LOG_LEVELS = [
  "DEBUG",
  "OK",
  "INFO",
  "ERROR",
  "WARNING",
  "CRITICAL",
  "EMERGENCY",
];

export const STATUS_LEVELS = ["OK", "INFO", "WARNING", "CRITICAL", "ERROR"];

/**
 * Niveau des probes désactivées. Elles ne sont pas exécutées, n'ont donc aucun
 * statut, et ne font pas partie de STATUS_LEVELS : elles ne sont ni comptées
 * ni filtrables par niveau.
 */
export const DISABLED_LEVEL = "DISABLED";

/**
 * Convertit un code de statut numérique en niveau de statut
 * @param {number} status - Code de statut
 * @returns {string} Niveau de statut
 */
export function getLevel(status) {
  if (status < 100) {
    return "ERROR";
  } else if (status == 100) {
    return "OK";
  } else if (status < 200) {
    return "INFO";
  } else if (status < 300) {
    return "WARNING";
  } else if (status < 500) {
    return "CRITICAL";
  } else {
    return "ERROR";
  }
}

/**
 * Retourne la classe CSS Bootstrap pour le texte selon le niveau
 * @param {string} level - Niveau de statut
 * @returns {string} Classe CSS
 */
export function getTextLevelClass(level) {
  const classes = {
    DISABLED: "text-secondary",
    OK: "text-success",
    INFO: "text-primary",
    WARNING: "text-warning",
    CRITICAL: "text-danger",
    ERROR: "text-dark",
  };
  return classes[level] || "";
}

/**
 * Retourne la classe CSS Bootstrap pour le fond selon le niveau
 * @param {string} level - Niveau de statut
 * @returns {string} Classe CSS
 */
export function getBgLevelClass(level) {
  const classes = {
    DISABLED: "bg-secondary",
    OK: "bg-success",
    INFO: "bg-primary",
    WARNING: "bg-warning",
    CRITICAL: "bg-danger",
    ERROR: "bg-dark",
  };
  return classes[level] || "";
}

/**
 * Retourne la classe CSS Bootstrap pour le badge selon le niveau
 * @param {string} level - Niveau de statut
 * @returns {string} Classe CSS
 */
export function getBadgeLevelClass(level) {
  const classes = {
    DISABLED: "badge bg-secondary",
    OK: "badge bg-success",
    INFO: "badge bg-primary",
    WARNING: "badge bg-warning",
    CRITICAL: "badge bg-danger",
    ERROR: "badge bg-dark",
  };
  return classes[level] || "badge bg-secondary";
}

/**
 * Retourne la classe CSS Bootstrap pour le bouton selon le niveau
 * @param {string} level - Niveau de statut
 * @returns {string} Classe CSS
 */
export function getBtnLevelClass(level) {
  const classes = {
    OK: "btn-success",
    INFO: "btn-info",
    WARNING: "btn-warning",
    CRITICAL: "btn-danger",
    ERROR: "btn-dark",
  };
  return classes[level] || "btn-secondary";
}

/**
 * Retourne la classe CSS Bootstrap pour un bouton en contour selon le niveau,
 * utilisée pour les niveaux désactivés dans les filtres
 * @param {string} level - Niveau de statut
 * @returns {string} Classe CSS
 */
export function getBtnOutlineLevelClass(level) {
  const classes = {
    OK: "btn-outline-success",
    INFO: "btn-outline-info",
    WARNING: "btn-outline-warning",
    CRITICAL: "btn-outline-danger",
    ERROR: "btn-outline-dark",
  };
  return classes[level] || "btn-outline-secondary";
}

/**
 * Retourne la classe CSS Bootstrap pour une ligne de tableau selon le statut
 * @param {number} status - Code de statut
 * @returns {string} Classe CSS
 */
export function getStatusRowClass(status) {
  const level = getLevel(status);
  const classes = {
    OK: "table-success",
    INFO: "table-info",
    WARNING: "table-warning",
    CRITICAL: "table-danger",
    ERROR: "table-dark",
  };
  return classes[level] || "";
}

/**
 * Retourne la classe CSS Bootstrap pour une ligne de log selon le niveau
 * @param {number} logLevel - Index du niveau de log (1-based)
 * @returns {string} Classe CSS
 */
export function getLogRowClass(logLevel) {
  const level = LOG_LEVELS[logLevel - 1];
  const classes = {
    DEBUG: "",
    OK: "table-success",
    INFO: "table-info",
    WARNING: "table-warning",
    CRITICAL: "table-danger",
    ERROR: "table-dark",
    EMERGENCY: "table-dark",
  };
  return classes[level] || "";
}

/**
 * Filtre les logs selon un niveau minimum
 * @param {Array} logs - Liste des logs
 * @param {string} minLevel - Niveau minimum
 * @returns {Array} Logs filtrés
 */
export function filterLogsByLevel(logs, minLevel) {
  const minIndex = LOG_LEVELS.indexOf(minLevel);
  if (minIndex === -1) return logs;
  return logs.filter((log) => log.Level >= minIndex + 1);
}
