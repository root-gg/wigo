<template>
  <div ref="root" class="status-timeline">
    <div class="d-flex flex-wrap align-items-center gap-2 mb-1">
      <div class="btn-group btn-group-sm" role="group" aria-label="Time range">
        <button
          v-for="range in RANGES"
          :key="range.seconds"
          type="button"
          :class="[
            'btn',
            range.seconds === seconds
              ? 'btn-secondary'
              : 'btn-outline-secondary',
          ]"
          @click="select(range.seconds)"
        >
          {{ range.label }}
        </button>
      </div>

      <span
        v-if="loaded && spans.length"
        class="small text-body-secondary ms-2"
      >
        {{ summary }}
      </span>
    </div>

    <p v-if="error" class="small text-body-secondary mb-0">
      <i class="fas fa-fw fa-circle-info"></i> {{ error }}
    </p>

    <p
      v-else-if="loaded && !spans.length"
      class="small text-body-secondary mb-0"
    >
      <i class="fas fa-fw fa-circle-info"></i>
      Nothing is known about this over that window.
      <span v-if="historyDays === 0">
        Status changes are not kept on this host: set StatusHistoryDays.
      </span>
    </p>

    <div v-else-if="loaded" class="position-relative">
      <!-- Une frise, pas une courbe : un statut est un état qui dure, et ce
           qu'on lit ici est combien de temps il a duré, pas sa valeur. -->
      <svg
        ref="plot"
        class="timeline-plot"
        :viewBox="`0 0 ${width} ${HEIGHT}`"
        role="img"
        :aria-label="ariaLabel"
        @pointermove="track"
        @pointerleave="hovered = null"
      >
        <defs>
          <!-- Rien de connu : ni bon ni mauvais. Une teinte de plus se lirait
               comme un statut, des hachures se lisent comme une absence. -->
          <pattern
            :id="`absent-${uid}`"
            width="6"
            height="6"
            patternUnits="userSpaceOnUse"
            patternTransform="rotate(45)"
          >
            <rect width="6" height="6" class="absent-ground" />
            <line x1="0" y1="0" x2="0" y2="6" class="absent-stroke" />
          </pattern>
        </defs>

        <rect
          v-for="span in spans"
          :key="span.at"
          :x="xAt(span.at)"
          :width="Math.max(1, xAt(span.until) - xAt(span.at))"
          y="0"
          :height="BAND"
          :class="['span', `level-${span.level}`]"
          :fill="span.status === ABSENT ? `url(#absent-${uid})` : undefined"
        />

        <!-- Le contour, pour que l'étendue de la bande reste lisible même
             quand on ne connaît rien de la fenêtre -->
        <rect
          class="band-outline"
          x="0.5"
          y="0.5"
          :width="Math.max(1, width - 1)"
          :height="BAND - 1"
        />

        <text
          v-for="tick in xTicks"
          :key="tick.at"
          class="axis"
          :x="tick.x"
          :y="HEIGHT - 4"
          text-anchor="middle"
        >
          {{ tick.label }}
        </text>

        <line
          v-if="hovered"
          class="crosshair"
          :x1="hovered.x"
          :x2="hovered.x"
          y1="0"
          :y2="BAND"
        />
      </svg>

      <div
        v-if="hovered"
        ref="tooltip"
        class="timeline-tooltip card shadow-sm"
        :style="tooltipStyle"
      >
        <div class="card-body p-2 small">
          <div class="d-flex align-items-center gap-2">
            <StatusBadge :level="hovered.span.level" size="sm">
              {{ hovered.span.status === ABSENT ? "—" : hovered.span.status }}
            </StatusBadge>
            <strong>{{ describeLevel(hovered.span) }}</strong>
          </div>
          <div class="text-body-secondary mt-1">
            {{ formatTime(hovered.span.at) }} · lasted
            {{ humanizeDuration(hovered.span.until - hovered.span.at) }}
          </div>
          <div v-if="hovered.span.message" class="mt-1">
            {{ hovered.span.message }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from "vue";
import api from "../api/client.js";
import StatusBadge from "./StatusBadge.vue";
import { getLevel } from "../utils/status.js";
import { humanizeDuration } from "../utils/disable.js";

const HEIGHT = 34;
const BAND = 16;
const PADDING = { left: 0, right: 0 };

const RANGES = [
  { label: "6h", seconds: 6 * 3600 },
  { label: "24h", seconds: 24 * 3600 },
  { label: "7d", seconds: 7 * 24 * 3600 },
  { label: "30d", seconds: 30 * 24 * 3600 },
];

/** Ce que le serveur renvoie pour « n'existait pas », cf. StatusAbsent */
const ABSENT = -1;

const props = defineProps({
  hostName: { type: String, required: true },
  /** Une sonde, ou vide pour le host lui-même */
  probeName: { type: String, default: "" },
});

/** Deux frises sur la même page ne doivent pas partager un id de pattern */
const uid = `${Math.random().toString(36).slice(2, 8)}`;

const root = ref(null);
const plot = ref(null);
const width = ref(720);
const seconds = ref(24 * 3600);
const timeline = ref(null);
const historyDays = ref(null);
const loaded = ref(false);
const error = ref("");
const hovered = ref(null);

const window_ = computed(() => {
  const until = timeline.value?.Until || Math.floor(Date.now() / 1000);
  const since = timeline.value?.Since || until - seconds.value;
  return { since, until };
});

/**
 * Les segments, chacun s'étendant jusqu'au changement suivant.
 *
 * Le dernier va jusqu'au bord droit : il n'a pas encore pris fin, et le couper
 * au dernier changement laisserait un blanc qui se lit comme « on ne sait
 * pas » alors qu'on sait très bien.
 */
const spans = computed(() => {
  if (!timeline.value) return [];

  const { since, until } = window_.value;
  const changes = timeline.value.Changes || [];
  const built = [];

  let status = timeline.value.StatusAtStart;
  let at = since;
  let message = "";

  for (const change of changes) {
    if (change.At > at) {
      built.push({ at, until: change.At, status, message });
    }
    status = change.Now;
    at = change.At;
    message = change.Message;
  }

  built.push({ at, until, status, message });

  return built
    .filter((span) => span.until > span.at)
    .map((span) => ({
      ...span,
      level: span.status === ABSENT ? "DISABLED" : getLevel(span.status),
    }));
});

/**
 * Le temps passé hors du vert, qui est la question qu'on se pose vraiment.
 *
 * Et ce qu'on ne sait pas, quand il y en a : une sonde installée hier a une
 * bande grise sur presque toute une fenêtre de 30 jours, et dire « fine the
 * whole time » de la journée qu'on connaît serait un mensonge sur les 29
 * autres.
 */
const summary = computed(() => {
  const { since, until } = window_.value;
  const total = until - since;
  if (total <= 0) return "";

  let bad = 0;
  let known = 0;
  for (const span of spans.value) {
    const length = span.until - span.at;
    if (span.status === ABSENT) continue;
    known += length;
    if (span.status !== 100) bad += length;
  }

  if (known === 0) return "nothing known over that window";

  // Relatif à la fenêtre : quelques minutes manquantes sur trente jours ne sont
  // pas un trou, et un seuil absolu ferait dire « only 24 hours known » d'une
  // fenêtre de 24 heures. Au-delà on annonce d'abord ce qu'on connaît, parce
  // que « fine the whole time » d'une fenêtre qu'on ignore aux trois quarts se
  // lit comme une garantie.
  if (total - known > Math.max(60, total * 0.02)) {
    const scope = `only ${humanizeDuration(known)} known`;

    return bad === 0
      ? `${scope}, fine throughout`
      : `${scope}, not fine for ${humanizeDuration(bad)} of it`;
  }

  return bad === 0
    ? "fine the whole time"
    : `not fine for ${humanizeDuration(bad)} of it`;
});

const ariaLabel = computed(() =>
  props.probeName
    ? `Status of ${props.probeName} over the last ${currentLabel.value}`
    : `Status of ${props.hostName} over the last ${currentLabel.value}`,
);

const currentLabel = computed(
  () => RANGES.find((range) => range.seconds === seconds.value)?.label || "",
);

function xAt(at) {
  const { since, until } = window_.value;
  const usable = width.value - PADDING.left - PADDING.right;
  const ratio = (at - since) / (until - since || 1);

  return PADDING.left + Math.min(Math.max(ratio, 0), 1) * usable;
}

const xTicks = computed(() => {
  const { since, until } = window_.value;
  const ticks = [];

  for (let i = 0; i <= 4; i++) {
    const at = since + ((until - since) * i) / 4;
    ticks.push({
      at,
      // Les extrêmes rentrées, sinon la première et la dernière débordent
      x: Math.min(Math.max(xAt(at), 26), width.value - 26),
      label: formatTime(at),
    });
  }

  return ticks;
});

const tooltip = ref(null);
const tooltipWidth = ref(0);

// Mesurée après le rendu : sa largeur vient de son contenu, et le contenu
// change à chaque segment survolé.
watch(hovered, async () => {
  await nextTick();
  tooltipWidth.value = tooltip.value ? tooltip.value.offsetWidth : 0;
});

/**
 * Centrée sur le curseur, et rentrée dans le cadre.
 *
 * On calcule en pixels plutôt que de basculer d'un côté à l'autre au-delà d'un
 * seuil : sur un écran étroit l'infobulle fait toute la largeur du cadre, et
 * aucun seuil ne la fait tenir. La ramener dans les bornes est la seule règle
 * qui marche aux deux tailles.
 *
 * Rien ici ne touche à sa largeur -- c'était le défaut d'avant, où un bloc posé
 * sur `left` sans largeur se faisait comprimer par ce qui restait à sa droite.
 */
const tooltipStyle = computed(() => {
  if (!hovered.value) return {};

  const tip = tooltipWidth.value;
  const room = width.value;

  // Tant qu'on ne l'a pas mesurée, on la pose au curseur : un instant à la
  // mauvaise place vaut mieux qu'un saut depuis le coin.
  if (!tip) return { left: `${hovered.value.x}px` };

  const wanted = hovered.value.x - tip / 2;
  const left = Math.max(0, Math.min(wanted, room - tip));

  return { left: `${left}px` };
});

function describeLevel(span) {
  if (span.status === ABSENT) return "not being watched";
  if (span.status === 100) return "OK";

  return span.level;
}

/** On vise un moment, pas un segment de deux pixels */
function track(event) {
  if (!plot.value || !spans.value.length) return;

  const box = plot.value.getBoundingClientRect();
  const ratio = (event.clientX - box.left) / box.width;
  const x = ratio * width.value;

  const { since, until } = window_.value;
  const at = since + ratio * (until - since);

  const span =
    spans.value.find((one) => at >= one.at && at < one.until) ||
    spans.value[spans.value.length - 1];

  hovered.value = { x, span };
}

function formatTime(at) {
  const date = new Date(at * 1000);

  if (seconds.value > 86400) {
    return date.toLocaleDateString([], { day: "2-digit", month: "2-digit" });
  }

  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function select(value) {
  seconds.value = value;
  load();
}

async function load() {
  error.value = "";
  try {
    const answer = await api.getHostTimeline(
      props.hostName,
      props.probeName,
      Math.floor(Date.now() / 1000) - seconds.value,
    );
    timeline.value = answer;
    historyDays.value = answer.HistoryDays;
    loaded.value = true;
  } catch (requestError) {
    error.value =
      requestError.response?.data ||
      requestError.message ||
      "The timeline could not be read";
    loaded.value = true;
  }
}

watch(() => [props.hostName, props.probeName], load);

let observer = null;

onMounted(() => {
  load();

  if (root.value && typeof ResizeObserver !== "undefined") {
    observer = new ResizeObserver(([entry]) => {
      width.value = Math.max(320, Math.round(entry.contentRect.width));
    });
    observer.observe(root.value);
  }
});

onUnmounted(() => {
  if (observer) observer.disconnect();
});
</script>

<style scoped>
.timeline-plot {
  width: 100%;
  height: 34px;
  display: block;
  touch-action: none;
}

/* Les couleurs de statut de wigo, pas la palette catégorielle : ici la couleur
   veut dire bon ou mauvais, elle ne distingue pas des identités. */
.span {
  shape-rendering: crispEdges;
}
.span.level-OK {
  fill: var(--bs-success);
}
.span.level-INFO {
  fill: var(--bs-info);
}
.span.level-WARNING {
  fill: var(--bs-warning);
}
.span.level-CRITICAL {
  fill: var(--bs-danger);
}
.span.level-ERROR {
  fill: var(--bs-dark);
}
/* Rien de connu : hachuré, cf. le pattern. Le fill est posé sur l'élément. */
.absent-ground {
  fill: var(--bs-secondary-bg);
}
.absent-stroke {
  stroke: var(--bs-border-color);
  stroke-width: 2;
}

.band-outline {
  fill: none;
  stroke: var(--bs-border-color);
  stroke-width: 1;
}

.crosshair {
  stroke: var(--bs-body-color);
  stroke-width: 1;
}

.axis {
  fill: var(--bs-secondary-color);
  font-size: 11px;
}

.timeline-tooltip {
  position: absolute;
  top: 2rem;
  pointer-events: none;
  z-index: 5;

  /* Sa taille vient de son contenu, pas de sa position.
     
     Sans largeur, un bloc absolu posé sur `left` se fait comprimer dans ce qui
     reste à sa droite : la même infobulle faisait 64px de haut à gauche de la
     frise et 152 à droite, le texte s'enroulant de plus en plus. Le
     `translate(-100%)` la déplace bien de l'autre côté, mais un transform
     n'affecte pas la mise en page -- la largeur était déjà écrasée. */
  width: max-content;
  min-width: 12rem;
  max-width: min(24rem, 100%);
}
</style>
