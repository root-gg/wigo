<template>
  <div class="d-flex align-items-center gap-2">
    <!-- Ce badge est la moitié importante de la fonctionnalité. Sans lui, une
         alerte muette est indiscernable d'une alerte que personne n'a vue. -->
    <span
      v-if="suppression"
      :class="[
        'badge',
        suppression.Kind === 'ack' ? 'text-bg-info' : 'text-bg-secondary',
      ]"
      :title="describeSuppression(suppression)"
    >
      <i
        :class="[
          'fas',
          'fa-fw',
          suppression.Kind === 'ack' ? 'fa-user-check' : 'fa-bell-slash',
        ]"
      ></i>
      {{ suppression.Kind === "ack" ? "acked" : "silenced" }}
    </span>

    <div v-if="editable" class="dropdown" @click.stop>
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
        :title="
          suppression
            ? 'Notifications are held back, put them back on'
            : 'Stop notifying about this'
        "
      >
        <span
          v-if="busy"
          class="spinner-border spinner-border-sm"
          role="status"
        ></span>
        <i v-else class="fas fa-fw fa-bell-slash"></i>
      </button>

      <ul class="dropdown-menu dropdown-menu-end">
        <li v-if="suppression">
          <a class="dropdown-item" href="#" @click.prevent="lift">
            <i class="fas fa-fw fa-bell"></i> Notify me again
          </a>
        </li>
        <li v-if="suppression"><hr class="dropdown-divider" /></li>

        <li class="px-3 pb-2" style="min-width: 17rem">
          <input
            v-model="reason"
            type="text"
            class="form-control form-control-sm mb-2"
            placeholder="Why? e.g. migrating this database"
            aria-label="Why notifications are being held back"
          />

          <!-- Acquitter, c'est dire « je sais, je m'en occupe » : ça n'a de
               sens que sur quelque chose qui va mal, et ça ne survit ni à une
               aggravation ni à un retour à la normale. -->
          <button
            v-if="ackable"
            class="btn btn-sm btn-info w-100 mb-2"
            type="button"
            @click="ack"
          >
            <i class="fas fa-fw fa-user-check"></i>
            I know, I am on it
          </button>

          <div class="input-group input-group-sm">
            <select
              v-model="duration"
              class="form-select"
              aria-label="How long to stay quiet"
            >
              <option v-for="choice in DURATIONS" :key="choice" :value="choice">
                {{ choice }}
              </option>
            </select>
            <button
              class="btn btn-outline-secondary"
              type="button"
              @click="silence"
            >
              Silence
            </button>
          </div>
        </li>

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
                error ? 'fa-triangle-exclamation' : 'fa-circle-info',
              ]"
            ></i>
            {{ error || notice }}
          </div>
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from "vue";
import api from "../api/client.js";
import { describeSuppression } from "../utils/suppression.js";

const DURATIONS = ["1h", "2h", "4h", "8h", "24h", "168h"];

const props = defineProps({
  /** Le host visé, ou le groupe quand scope vaut "group" */
  target: {
    type: String,
    required: true,
  },
  scope: {
    type: String,
    default: "host",
  },
  /** Une probe du host, ou vide pour le host entier */
  probeName: {
    type: String,
    default: "",
  },
  /** La suppression qui couvre exactement cette cible, ou null */
  suppression: {
    type: Object,
    default: null,
  },
  /**
   * Le statut courant de la cible. Acquitter ce qui va bien n'a pas de sens,
   * et l'API le refuserait de toute façon.
   */
  status: {
    type: Number,
    default: 100,
  },
  editable: {
    type: Boolean,
    default: false,
  },
  onColor: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["changed"]);

const busy = ref(false);
const error = ref("");
const notice = ref("");
const reason = ref("");
const duration = ref("2h");

// Un groupe n'a pas de statut unique, donc rien à acquitter : quarante hosts
// ne sont pas « une chose » dont on dit qu'on s'en occupe.
const ackable = computed(() => props.scope === "host" && props.status > 100);

async function run(action) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    emit("changed", await action());
    reason.value = "";
  } catch (requestError) {
    error.value =
      requestError.response?.data ||
      requestError.message ||
      "The request failed";
  } finally {
    busy.value = false;
  }
}

function ack() {
  return run(() =>
    api.ackHost(props.target, props.probeName, reason.value.trim()),
  );
}

function silence() {
  if (props.scope === "group") {
    return run(() =>
      api.silenceGroup(props.target, duration.value, reason.value.trim()),
    );
  }

  return run(() =>
    api.silenceHost(
      props.target,
      props.probeName,
      duration.value,
      reason.value.trim(),
    ),
  );
}

function lift() {
  if (props.scope === "group") {
    return run(() => api.unsuppressGroup(props.target));
  }

  return run(() => api.unsuppressHost(props.target, props.probeName));
}
</script>
