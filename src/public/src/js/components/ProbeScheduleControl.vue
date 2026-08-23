<template>
  <div class="d-flex align-items-center gap-2">
    <span
      v-if="schedule"
      :class="[
        'badge',
        schedule.Enabled ? 'text-bg-light' : 'text-bg-secondary',
      ]"
      :title="
        schedule.Enabled
          ? `Runs every ${formatInterval(schedule.Interval)}`
          : 'Disabled: this probe is not executed at all'
      "
    >
      <i
        :class="['fas', 'fa-fw', schedule.Enabled ? 'fa-clock' : 'fa-ban']"
      ></i>
      {{ schedule.Enabled ? formatInterval(schedule.Interval) : "disabled" }}
    </span>

    <!-- Installée à plusieurs intervalles : elle tourne plusieurs fois par
         cycle et l'API refuse d'y toucher. Proposer un réglage qui échouera à
         coup sûr serait pire que de dire ce qui ne va pas. -->
    <span
      v-if="ambiguous"
      class="badge text-bg-warning"
      :title="`Installed in ${schedule.Directories.join(' and ')} at once, so it runs several times per cycle. Resolve it in the probes directory before changing it from here.`"
    >
      <i class="fas fa-fw fa-triangle-exclamation"></i>
      installed {{ schedule.Directories.length }}&times;
    </span>

    <div v-else-if="editable" class="dropdown" @click.stop>
      <button
        :class="[
          'btn',
          'btn-sm',
          'dropdown-toggle',
          onColor ? 'btn-light' : 'btn-outline-secondary',
        ]"
        type="button"
        data-bs-toggle="dropdown"
        data-bs-auto-close="outside"
        aria-expanded="false"
        :disabled="busy"
        title="Change how often this probe runs"
      >
        <span
          v-if="busy"
          class="spinner-border spinner-border-sm"
          role="status"
        ></span>
        <i v-else class="fas fa-fw fa-sliders"></i>
      </button>

      <ul class="dropdown-menu dropdown-menu-end">
        <li>
          <h6 class="dropdown-header">Run every</h6>
        </li>
        <li v-for="preset in INTERVAL_PRESETS" :key="preset">
          <a
            :class="[
              'dropdown-item',
              { active: schedule?.Enabled && schedule.Interval === preset },
            ]"
            href="#"
            @click.prevent="apply(preset)"
          >
            {{ formatInterval(preset) }}
          </a>
        </li>

        <li><hr class="dropdown-divider" /></li>
        <li class="px-3 pb-2">
          <!-- novalidate : sans ça le navigateur refuse l'envoi hors bornes
               sans rien dire de lisible, et notre propre message n'apparaît
               jamais -->
          <form
            class="input-group input-group-sm"
            novalidate
            @submit.prevent="applyCustom"
          >
            <input
              v-model="custom"
              type="number"
              class="form-control"
              :min="MIN_INTERVAL"
              :max="MAX_INTERVAL"
              placeholder="seconds"
              aria-label="Custom interval in seconds"
            />
            <button class="btn btn-outline-secondary" type="submit">Set</button>
          </form>
        </li>

        <li><hr class="dropdown-divider" /></li>
        <li>
          <a
            v-if="schedule?.Enabled"
            class="dropdown-item text-danger"
            href="#"
            @click.prevent="disable"
          >
            <i class="fas fa-fw fa-ban"></i> Disable this probe
          </a>
          <span v-else class="dropdown-item-text text-body-secondary small">
            Pick an interval above to enable it again
          </span>
        </li>

        <!-- Le message vit dans le menu : le slot qui accueille ce composant
             est une ligne flex d'en-tête de carte, où un bloc d'erreur serait
             invisible. Le menu reste ouvert après une action, donc un refus
             venu de l'API se voit aussi. -->
        <li v-if="error || notice">
          <hr class="dropdown-divider" />
          <div
            :class="[
              'px-3',
              'pb-2',
              'small',
              error ? 'text-danger' : 'text-body-secondary',
            ]"
            role="alert"
          >
            <i
              :class="[
                'fas',
                'fa-fw',
                error ? 'fa-triangle-exclamation' : 'fa-hourglass-half',
              ]"
            ></i>
            {{ error || notice }}
          </div>
        </li>
      </ul>
    </div>

    <span
      v-else-if="schedule && !ambiguous"
      :class="onColor ? 'text-white' : 'text-body-secondary'"
      :title="readOnlyReason"
    >
      <i class="fas fa-fw fa-lock"></i>
    </span>
  </div>
</template>

<script setup>
import { computed, ref } from "vue";
import api from "../api/client.js";

const MIN_INTERVAL = 2;
const MAX_INTERVAL = 86400;
const INTERVAL_PRESETS = [60, 120, 300, 600, 900, 3600];

const props = defineProps({
  hostName: {
    type: String,
    required: true,
  },
  probeName: {
    type: String,
    required: true,
  },
  /** Entrée de /api/probes pour cette probe, ou null si inconnue */
  schedule: {
    type: Object,
    default: null,
  },
  /** Faux quand le host n'est pas celui qui sert l'interface, ou en lecture seule */
  editable: {
    type: Boolean,
    default: false,
  },
  readOnlyReason: {
    type: String,
    default: "",
  },
  /**
   * Vrai quand ce contrôle est posé sur une surface colorée, l'en-tête d'une
   * StatusCard : un bouton en contour gris y disparaît presque, alors qu'il est
   * le plus lisible sur le fond neutre d'un tableau.
   */
  onColor: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["changed"]);

const busy = ref(false);
const error = ref("");
const notice = ref("");
const custom = ref("");

const ambiguous = computed(
  () => (props.schedule?.Directories?.length || 1) > 1,
);

function formatInterval(seconds) {
  if (!seconds) return "—";
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}min`;
  return `${seconds}s`;
}

async function run(action) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    const answer = await action();

    // Un host qui pousse vers ce master ne peut pas être appelé : l'ordre est
    // mis en file et l'API répond une phrase, pas l'ordonnancement. Rien n'a
    // encore bougé, donc il ne faut surtout pas l'afficher comme appliqué.
    if (typeof answer !== "object" || answer === null) {
      notice.value = String(answer);
      return;
    }

    emit("changed", answer);
  } catch (requestError) {
    // The API answers with a plain sentence explaining the refusal, which is
    // far more useful than a status code.
    error.value =
      requestError.response?.data ||
      requestError.message ||
      "The request failed";
  } finally {
    busy.value = false;
  }
}

function apply(seconds) {
  return run(() =>
    api.setHostProbeInterval(props.hostName, props.probeName, seconds),
  );
}

function applyCustom() {
  const seconds = parseInt(custom.value, 10);

  // The same bounds the API enforces, repeated here only to say why in place
  // rather than leave the browser to refuse the submit without a word.
  if (Number.isNaN(seconds)) {
    error.value = "Enter an interval in seconds";
    return;
  }
  if (seconds < MIN_INTERVAL || seconds > MAX_INTERVAL) {
    error.value = `Interval must be between ${MIN_INTERVAL} and ${MAX_INTERVAL} seconds`;
    return;
  }

  custom.value = "";
  return apply(seconds);
}

function disable() {
  return run(() => api.disableHostProbe(props.hostName, props.probeName));
}
</script>
