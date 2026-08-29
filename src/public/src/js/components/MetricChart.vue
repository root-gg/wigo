<template>
  <div ref="root" class="metric-chart">
    <!-- Les filtres sur une ligne au-dessus du graphe -->
    <div class="d-flex flex-wrap align-items-center gap-2 mb-2">
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

      <button
        type="button"
        class="btn btn-sm btn-outline-secondary ms-auto"
        :aria-pressed="asTable"
        @click="asTable = !asTable"
      >
        <i
          class="fas fa-fw"
          :class="asTable ? 'fa-chart-line' : 'fa-table'"
        ></i>
        {{ asTable ? "Chart" : "Table" }}
      </button>
    </div>

    <p v-if="error" class="small text-body-secondary mb-0">
      <i class="fas fa-fw fa-circle-info"></i> {{ error }}
    </p>

    <p
      v-else-if="loaded && !series.length"
      class="small text-body-secondary mb-0"
    >
      <i class="fas fa-fw fa-circle-info"></i>
      This probe reported no measurement over that window.
      <span v-if="retentionDays === 0">
        Nothing is kept on this host: set MetricsRetentionDays to keep a
        history.
      </span>
    </p>

    <template v-else-if="loaded">
      <!-- La vue tableau : elle porte les valeurs que la couleur seule ne
           suffirait pas à distinguer, et elle est atteignable sans survol. -->
      <div v-if="asTable" class="table-responsive" style="max-height: 20rem">
        <table class="table table-sm table-bordered mb-0 small">
          <thead>
            <tr>
              <th>Time</th>
              <th v-for="one in visible" :key="one.key" class="text-end">
                {{ one.label }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(at, index) in axisTimes" :key="at">
              <td>{{ formatTime(at) }}</td>
              <td v-for="one in visible" :key="one.key" class="text-end">
                {{ formatValue(one.points[index]?.Value) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else class="position-relative">
        <svg
          ref="plot"
          class="metric-plot"
          :viewBox="`0 0 ${width} ${HEIGHT}`"
          role="img"
          :aria-label="`${probeName} over the last ${currentRange.label}`"
          @pointermove="track"
          @pointerleave="hovered = null"
        >
          <!-- Grille : filet d'un pas au-dessus du fond, jamais tiretée -->
          <g class="grid">
            <line
              v-for="tick in yTicks"
              :key="tick.value"
              :x1="PADDING.left"
              :x2="width - PADDING.right"
              :y1="tick.y"
              :y2="tick.y"
            />
          </g>

          <text
            v-for="tick in yTicks"
            :key="`label-${tick.value}`"
            class="axis"
            :x="PADDING.left - 6"
            :y="tick.y + 3"
            text-anchor="end"
          >
            {{ tick.label }}
          </text>

          <text
            v-for="tick in xTicks"
            :key="tick.at"
            class="axis"
            :x="tick.x"
            :y="HEIGHT - 6"
            text-anchor="middle"
          >
            {{ tick.label }}
          </text>

          <!-- Ce que la moyenne a caché. Seulement à une ou deux séries : au
               delà, les plages se recouvrent et n'apprennent plus rien. -->
          <template v-if="visible.length <= 2">
            <path
              v-for="one in visible"
              :key="`band-${one.key}`"
              class="band"
              :d="bandPath(one)"
              :fill="one.color"
            />
          </template>

          <path
            v-for="one in visible"
            :key="`line-${one.key}`"
            class="line"
            :d="linePath(one)"
            :stroke="one.color"
          />

          <template v-if="hovered !== null">
            <line
              class="crosshair"
              :x1="xAt(hovered)"
              :x2="xAt(hovered)"
              :y1="PADDING.top"
              :y2="HEIGHT - PADDING.bottom"
            />
            <circle
              v-for="one in visible"
              :key="`dot-${one.key}`"
              :cx="xAt(hovered)"
              :cy="yAt(one.points[hovered]?.Value)"
              r="4"
              :fill="one.color"
              class="dot"
            />
          </template>
        </svg>

        <!-- Un seul relevé, listant toutes les séries à cet instant -->
        <div
          v-if="hovered !== null"
          class="metric-tooltip card shadow-sm"
          :style="tooltipStyle"
        >
          <div class="card-body p-2 small">
            <div class="text-body-secondary mb-1">
              {{ formatTime(axisTimes[hovered]) }}
            </div>
            <div
              v-for="one in visible"
              :key="one.key"
              class="d-flex align-items-center gap-2"
            >
              <span class="swatch" :style="{ background: one.color }"></span>
              <strong>{{ formatValue(one.points[hovered]?.Value) }}</strong>
              <span class="text-body-secondary">{{ one.label }}</span>
              <span
                v-if="showsRange(one, hovered)"
                class="text-body-secondary ms-auto"
              >
                {{ formatValue(one.points[hovered].Min) }}–{{
                  formatValue(one.points[hovered].Max)
                }}
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- La légende porte des libellés visibles, ce qu'impose le contraste de
           trois teintes de la palette sur fond clair. Cliquer masque une série
           sans jamais repeindre les autres. -->
      <div v-if="series.length > 1" class="d-flex flex-wrap gap-3 mt-2">
        <button
          v-for="one in series"
          :key="one.key"
          type="button"
          class="legend-key small"
          :class="one.shown ? 'text-body' : 'text-body-secondary'"
          :aria-pressed="one.shown"
          @click="toggle(one)"
        >
          <span
            class="swatch"
            :style="{
              background: one.shown ? one.color : 'transparent',
              borderColor: one.color,
            }"
          ></span>
          {{ one.label }}
        </button>
      </div>

      <p v-if="folded" class="small text-body-secondary mt-1 mb-0">
        <i class="fas fa-fw fa-circle-info"></i>
        {{ folded }} more series measured over that window and not shown: past
        eight, another colour would be indistinguishable from one already used.
      </p>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from "vue";
import api from "../api/client.js";

const HEIGHT = 220;

// À droite, de quoi ne pas couper la dernière étiquette de temps, qui est
// centrée sur son point.
const PADDING = { top: 10, right: 34, bottom: 22, left: 46 };

const RANGES = [
  { label: "1h", seconds: 3600 },
  { label: "6h", seconds: 6 * 3600 },
  { label: "24h", seconds: 24 * 3600 },
  { label: "7d", seconds: 7 * 24 * 3600 },
];

/**
 * L'ordre des teintes est le mécanisme de sécurité pour les daltonismes, pas
 * une décoration : il a été validé pour les paires voisines sur les deux fonds
 * réels de wigo. Une neuvième série n'obtient pas une couleur engendrée.
 */
const MAX_SERIES = 8;

const props = defineProps({
  hostName: { type: String, required: true },
  probeName: { type: String, required: true },
});

const root = ref(null);
const plot = ref(null);

/**
 * La largeur suit le conteneur, une unité du viewBox pour un pixel. Étirer le
 * viewBox à la place déformerait le texte avec le tracé, ce qui se voit tout de
 * suite sur les graduations.
 */
const width = ref(720);
const raw = ref([]);
const hidden = ref(new Set());
const seconds = ref(3600);
const retentionDays = ref(null);
const loaded = ref(false);
const error = ref("");
const asTable = ref(false);
const hovered = ref(null);

const currentRange = computed(
  () => RANGES.find((range) => range.seconds === seconds.value) || RANGES[0],
);

function seriesLabel(entry) {
  const tags = Object.entries(entry.Tags || {});
  if (!tags.length) return props.probeName;

  return tags.map(([key, value]) => `${key}=${value}`).join(" ");
}

/**
 * La couleur suit la série, pas son rang dans ce qui reste affiché : masquer
 * une courbe ne doit jamais repeindre les autres. L'indice vient donc de
 * l'ordre stable renvoyé par l'API, jamais de la liste filtrée.
 */
const series = computed(() =>
  raw.value.slice(0, MAX_SERIES).map((entry, index) => {
    const key = seriesLabel(entry);
    return {
      key,
      label: key,
      slot: index + 1,
      color: `var(--wigo-series-${index + 1})`,
      points: entry.Points || [],
      shown: !hidden.value.has(key),
    };
  }),
);

const visible = computed(() => series.value.filter((one) => one.shown));
const folded = computed(() => Math.max(0, raw.value.length - MAX_SERIES));

/** L'axe des temps vient de la série la plus fournie */
const axisTimes = computed(() => {
  let longest = [];
  for (const one of series.value) {
    if (one.points.length > longest.length) longest = one.points;
  }
  return longest.map((point) => point.At);
});

const bounds = computed(() => {
  let low = Infinity;
  let high = -Infinity;

  for (const one of visible.value) {
    for (const point of one.points) {
      const min = visible.value.length <= 2 ? point.Min : point.Value;
      const max = visible.value.length <= 2 ? point.Max : point.Value;
      if (min < low) low = min;
      if (max > high) high = max;
    }
  }

  if (low === Infinity) return { low: 0, high: 1 };
  if (low === high) return { low: low - 1, high: high + 1 };

  // Un peu d'air en haut, et un zéro gardé quand la série y touche presque
  const margin = (high - low) * 0.1;
  return {
    low: low >= 0 && low - margin < 0 ? 0 : low - margin,
    high: high + margin,
  };
});

function xAt(index) {
  const count = Math.max(1, axisTimes.value.length - 1);
  const usable = width.value - PADDING.left - PADDING.right;

  return PADDING.left + (index / count) * usable;
}

function yAt(value) {
  const { low, high } = bounds.value;
  const usable = HEIGHT - PADDING.top - PADDING.bottom;
  const ratio = (Number(value) - low) / (high - low || 1);

  return HEIGHT - PADDING.bottom - ratio * usable;
}

function linePath(one) {
  return one.points
    .map(
      (point, index) => `${index ? "L" : "M"}${xAt(index)},${yAt(point.Value)}`,
    )
    .join(" ");
}

function bandPath(one) {
  const up = one.points.map(
    (point, index) => `${index ? "L" : "M"}${xAt(index)},${yAt(point.Max)}`,
  );
  const down = one.points
    .map((point, index) => `L${xAt(index)},${yAt(point.Min)}`)
    .reverse();

  return [...up, ...down, "Z"].join(" ");
}

/** Des nombres ronds, et jamais plus de cinq graduations */
const yTicks = computed(() => {
  const { low, high } = bounds.value;
  const step = niceStep((high - low) / 4);
  const ticks = [];

  for (let value = Math.ceil(low / step) * step; value <= high; value += step) {
    ticks.push({ value, y: yAt(value), label: formatValue(value) });
    if (ticks.length >= 6) break;
  }

  return ticks;
});

function niceStep(rough) {
  if (!isFinite(rough) || rough <= 0) return 1;

  const magnitude = Math.pow(10, Math.floor(Math.log10(rough)));
  const normalised = rough / magnitude;
  const step =
    normalised <= 1 ? 1 : normalised <= 2 ? 2 : normalised <= 5 ? 5 : 10;

  return step * magnitude;
}

const xTicks = computed(() => {
  const times = axisTimes.value;
  if (times.length < 2) return [];

  const wanted = 5;
  const every = Math.max(1, Math.floor(times.length / wanted));
  const ticks = [];

  for (let index = 0; index < times.length; index += every) {
    ticks.push({
      at: times[index],
      x: xAt(index),
      label: formatTime(times[index]),
    });
  }

  return ticks;
});

const tooltipStyle = computed(() => {
  const ratio = xAt(hovered.value) / width.value;

  return {
    left: `${ratio * 100}%`,
    transform: ratio > 0.6 ? "translate(-100%, 0)" : "translate(0, 0)",
  };
});

function showsRange(one, index) {
  const point = one.points[index];

  return point && (point.Min !== point.Value || point.Max !== point.Value);
}

/**
 * Le viseur trouve l'abscisse : on vise un instant, jamais un trait de deux
 * pixels. Il se cale sur le point le plus proche.
 */
function track(event) {
  if (!plot.value || !axisTimes.value.length) return;

  const box = plot.value.getBoundingClientRect();
  const ratio = (event.clientX - box.left) / box.width;
  const x = ratio * width.value;

  const count = Math.max(1, axisTimes.value.length - 1);
  const usable = width.value - PADDING.left - PADDING.right;
  const index = Math.round(((x - PADDING.left) / usable) * count);

  hovered.value = Math.min(Math.max(index, 0), axisTimes.value.length - 1);
}

function toggle(one) {
  const next = new Set(hidden.value);
  if (next.has(one.key)) {
    next.delete(one.key);
  } else if (visible.value.length > 1) {
    // Tout masquer ne laisserait rien à lire
    next.add(one.key);
  }
  hidden.value = next;
}

function select(value) {
  seconds.value = value;
  load();
}

function formatValue(value) {
  if (value === undefined || value === null || !isFinite(value)) return "—";
  if (Math.abs(value) >= 1000) return Math.round(value).toLocaleString();
  if (Math.abs(value) >= 10) return value.toFixed(1);

  return value.toFixed(2);
}

/** Ce que les points couvrent vraiment, qui n'est pas ce qui a été demandé */
const covered = computed(() => {
  const times = axisTimes.value;
  return times.length > 1 ? times[times.length - 1] - times[0] : 0;
});

function formatTime(at) {
  if (!at) return "";
  const date = new Date(at * 1000);

  // Au delà d'une journée, l'heure seule ne situe plus un point. En deçà de dix
  // minutes, la minute seule donne cinq fois la même étiquette.
  if (covered.value > 86400) {
    return date.toLocaleString([], {
      day: "2-digit",
      month: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  }
  if (covered.value < 600) {
    return date.toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  }

  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

async function load() {
  error.value = "";
  try {
    const answer = await api.getProbeMetrics(
      props.hostName,
      props.probeName,
      Math.floor(Date.now() / 1000) - seconds.value,
    );
    raw.value = answer.Series || [];
    retentionDays.value = answer.RetentionDays;
    loaded.value = true;
  } catch (requestError) {
    // Un host qui pousse ne peut pas être interrogé, un wigo trop ancien n'a
    // pas cet endpoint : dans les deux cas il le dit en clair.
    error.value =
      requestError.response?.data ||
      requestError.message ||
      "The history could not be read";
    loaded.value = true;
  }
}

watch(() => [props.hostName, props.probeName], load);

let observer = null;

onMounted(() => {
  load();

  // Le conteneur, pas le svg : celui-ci n'existe pas encore au montage, le
  // chargement étant asynchrone, et l'observateur ne se serait jamais attaché.
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
/*
 * Les teintes viennent de la palette catégorielle validée, dans son ordre :
 * cet ordre est le mécanisme de sécurité pour les daltonismes, pas un choix
 * esthétique. Validé contre les deux fonds réels de wigo, le blanc des cartes
 * en clair et le #212529 de Bootstrap en sombre.
 */
.metric-chart {
  --wigo-series-1: #2a78d6;
  --wigo-series-2: #eb6834;
  --wigo-series-3: #1baf7a;
  --wigo-series-4: #eda100;
  --wigo-series-5: #e87ba4;
  --wigo-series-6: #008300;
  --wigo-series-7: #4a3aa7;
  --wigo-series-8: #e34948;
}

/* Les mêmes huit teintes, remarchées pour le fond sombre, pas une autre palette */
:global([data-bs-theme="dark"]) .metric-chart {
  --wigo-series-1: #3987e5;
  --wigo-series-2: #d95926;
  --wigo-series-3: #199e70;
  --wigo-series-4: #c98500;
  --wigo-series-5: #d55181;
  --wigo-series-6: #008300;
  --wigo-series-7: #9085e9;
  --wigo-series-8: #e66767;
}

.metric-plot {
  width: 100%;
  height: 220px;
  display: block;
  touch-action: none;
}

/* Un filet d'un pas au-dessus du fond, jamais tireté : la grille se lit sans
   se faire remarquer. */
.grid line {
  stroke: var(--bs-border-color);
  stroke-width: 1;
}

.crosshair {
  stroke: var(--bs-secondary-color);
  stroke-width: 1;
}

.line {
  fill: none;
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
}

/* Ce que la moyenne a caché, en retrait derrière la courbe */
.band {
  opacity: 0.15;
  stroke: none;
}

/* Un anneau de la couleur du fond, pour que le point reste lisible là où il
   croise une courbe. */
.dot {
  stroke: var(--bs-body-bg);
  stroke-width: 2;
}

/* Le texte porte les jetons de texte, jamais la couleur d'une série */
.axis {
  fill: var(--bs-secondary-color);
  font-size: 11px;
}

/* Un bouton dépouillé plutôt qu'un btn-link : celui-ci impose sa propre
   couleur et écrase les jetons de texte, ce qui rendait le libellé illisible
   sur fond sombre -- or ces libellés sont le relief qu'impose le contraste de
   trois teintes de la palette sur fond clair. */
.legend-key {
  background: none;
  border: 0;
  padding: 0;
  cursor: pointer;
  color: inherit;
}

.swatch {
  display: inline-block;
  width: 0.65rem;
  height: 0.65rem;
  border-radius: 0.15rem;
  border: 2px solid transparent;
  vertical-align: -1px;
}

.metric-tooltip {
  position: absolute;
  top: 0.5rem;
  pointer-events: none;
  min-width: 12rem;
  z-index: 5;
}
</style>
