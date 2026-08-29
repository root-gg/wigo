<template>
  <div v-if="shape !== 'empty'" class="probe-detail">
    <div class="d-flex align-items-center justify-content-end gap-2 mb-2">
      <!-- Le JSON reste accessible même quand un tableau le remplace : c'est
           ce qu'on colle dans un ticket, et c'est la seule vue qui ne cache
           rien de ce que la sonde a rendu. -->
      <button
        v-if="shape !== 'raw'"
        type="button"
        class="btn btn-sm btn-outline-secondary"
        :aria-pressed="raw"
        @click="raw = !raw"
      >
        <i class="fas fa-fw fa-code"></i>
        {{ raw ? "Table" : "JSON" }}
      </button>

      <button
        type="button"
        class="btn btn-sm btn-outline-secondary"
        @click="copy"
      >
        <i :class="['fas', 'fa-fw', copied ? 'fa-check' : 'fa-copy']"></i>
        {{ copied ? "Copied" : "Copy" }}
      </button>
    </div>

    <!-- Un objet plat : ce que la sonde a mesuré, nom et valeur -->
    <div v-if="!raw && shape === 'pairs'" class="detail-scroll">
      <table class="table table-sm table-bordered mb-0">
        <tbody>
          <tr v-for="pair in pairs" :key="pair.key">
            <th class="w-25 text-nowrap">{{ pair.key }}</th>
            <td><DetailValue :value="pair.value" /></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Plusieurs choses décrites de la même façon : un disque, un device,
         un service. Vingt-trois devices en JSON font deux cents lignes que
         personne ne lit ; en lignes, ça se compare d'un coup d'oeil. -->
    <div v-else-if="!raw && shape === 'rows'" class="detail-scroll">
      <table class="table table-sm table-bordered table-hover mb-0">
        <thead>
          <tr>
            <th v-if="rows.labelled" class="text-nowrap"></th>
            <th v-for="column in columns" :key="column" class="text-nowrap">
              {{ column }}
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows.entries" :key="row.label">
            <th v-if="rows.labelled" class="text-nowrap">{{ row.label }}</th>
            <td v-for="column in columns" :key="column">
              <DetailValue :value="row.values[column]" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <pre v-else class="border rounded p-3 bg-body-tertiary detail-scroll">{{
      json
    }}</pre>
  </div>
</template>

<script setup>
import { ref, computed, watch } from "vue";
import DetailValue from "./DetailValue.vue";

const props = defineProps({
  detail: { type: null, default: null },
});

const raw = ref(false);
const copied = ref(false);

const json = computed(() => JSON.stringify(props.detail, null, 2));

function isScalar(value) {
  return (
    value === null ||
    ["string", "number", "boolean"].includes(typeof value) ||
    value === undefined
  );
}

/** Ce qu'une cellule peut porter sans mentir : un scalaire, ou une liste */
function isRenderable(value) {
  if (isScalar(value)) return true;

  return Array.isArray(value) && value.every(isScalar);
}

function isPlainObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

const pairs = computed(() => {
  if (!isPlainObject(props.detail)) return [];

  return Object.entries(props.detail).map(([key, value]) => ({ key, value }));
});

/**
 * Les lignes d'un tableau, quand le détail décrit plusieurs fois la même
 * chose : un objet d'objets ({"/dev/sda1": {...}, "overlay": {...}}) ou un
 * tableau d'objets. Le nom porté par la clé devient la première colonne ;
 * un tableau n'en a pas, ses index n'apprennent rien.
 */
const rows = computed(() => {
  const detail = props.detail;

  if (Array.isArray(detail)) {
    return {
      labelled: false,
      entries: detail.map((values, index) => ({ label: index, values })),
    };
  }
  if (isPlainObject(detail)) {
    return {
      labelled: true,
      entries: Object.entries(detail).map(([label, values]) => ({
        label,
        values,
      })),
    };
  }

  return { labelled: false, entries: [] };
});

/** L'union des clés, dans l'ordre où on les rencontre */
const columns = computed(() => {
  const seen = [];
  for (const entry of rows.value.entries) {
    for (const key of Object.keys(entry.values || {})) {
      if (!seen.includes(key)) seen.push(key);
    }
  }

  return seen;
});

/**
 * Un tableau, ou rien.
 *
 * Le seuil est volontairement franc : ou toutes les cellules sont lisibles,
 * ou on rend le JSON. Un tableau dont la moitié des cases contiennent un blob
 * JSON est moins lisible que le JSON entier, et laisse croire qu'on a tout vu.
 */
const shape = computed(() => {
  const detail = props.detail;

  if (detail === null || detail === undefined || detail === "") return "empty";
  if (isPlainObject(detail) && Object.keys(detail).length === 0) return "empty";
  if (Array.isArray(detail) && detail.length === 0) return "empty";
  if (isScalar(detail)) return "raw";

  const entries = rows.value.entries;

  if (entries.length && entries.every((one) => isPlainObject(one.values))) {
    const everyCellFits = entries.every((entry) =>
      columns.value.every((column) => isRenderable(entry.values[column])),
    );
    if (everyCellFits && columns.value.length) return "rows";
  }

  if (isPlainObject(detail) && Object.values(detail).every(isRenderable)) {
    return "pairs";
  }

  return "raw";
});

async function copy() {
  try {
    await navigator.clipboard.writeText(json.value);
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    // Pas de presse-papier : le JSON reste sélectionnable à la main
  }
}

// Une sonde qui change de forme ne doit pas garder la vue de la précédente
watch(shape, () => {
  raw.value = false;
});
</script>

<style scoped>
.detail-scroll {
  max-height: 400px;
  overflow: auto;
}

pre.detail-scroll {
  margin-bottom: 0;
}

/* Les en-têtes restent lisibles quand vingt-trois devices défilent dessous */
.probe-detail thead th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--bs-body-bg);
}
</style>
