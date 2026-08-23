/**
 * Mise en forme des décisions de désactivation.
 *
 * Une sonde désactivée est un angle mort qu'on s'est créé soi-même. Ce qui la
 * rend supportable est de savoir qui, quand, pourquoi et jusqu'à quand — donc
 * tout l'intérêt est dans la lisibilité de ces quatre phrases.
 */

const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** Indexe les enregistrements d'un ordonnancement par nom de sonde */
export function disableRecordsByProbe(schedule) {
  const byName = {};
  for (const record of schedule?.DisableRecords || []) {
    byName[record.Probe] = record;
  }
  return byName;
}

function plural(count, word) {
  return `${count} ${word}${count > 1 ? "s" : ""}`;
}

/**
 * Une durée approximative et lisible. La précision n'a aucun intérêt ici :
 * ce qui compte est de distinguer « cet après-midi » de « il y a huit mois ».
 */
export function humanizeDuration(seconds) {
  const value = Math.max(0, Math.round(seconds));

  if (value < MINUTE) return "less than a minute";
  if (value < HOUR) return plural(Math.round(value / MINUTE), "minute");
  if (value < DAY) return plural(Math.round(value / HOUR), "hour");
  if (value < 60 * DAY) return plural(Math.round(value / DAY), "day");

  return plural(Math.round(value / (30 * DAY)), "month");
}

export function formatTimestamp(timestamp) {
  if (!timestamp) return "";
  return new Date(timestamp * 1000).toLocaleString();
}

/** Depuis combien de temps la sonde est éteinte, en secondes */
export function disabledFor(record, now = Date.now() / 1000) {
  if (!record?.CreatedAt) return 0;
  return now - record.CreatedAt;
}

/** « disabled by X 3 days ago : reason » */
export function describeDisable(record) {
  if (!record) return "";

  const who = record.Author || "someone";
  let sentence = `Disabled by ${who} ${humanizeDuration(disabledFor(record))} ago`;

  if (record.Reason) {
    sentence += `: ${record.Reason}`;
  }

  return sentence;
}

/**
 * Ce qu'il reste avant le retour automatique, ou la phrase qui dit qu'il n'y en
 * a pas — c'est cette absence qui laisse une sonde éteinte pendant huit mois.
 */
export function describeExpiry(record) {
  if (!record) return "";

  if (!record.Until) {
    return "No end date: it stays disabled until someone enables it again.";
  }

  const remaining = record.Until - Date.now() / 1000;
  if (remaining <= 0) {
    return "Its deadline has passed, it comes back on the next check.";
  }

  return `Comes back on its own in ${humanizeDuration(remaining)}, on ${formatTimestamp(record.Until)}.`;
}
