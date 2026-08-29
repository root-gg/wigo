<template>
  <AppLayout
    :current-interval="interval"
    :filterable="false"
    title-context="API tokens"
    @refresh-settings="handleRefreshSettings"
  >
    <template #sidebar>
      <li class="nav-item sidebar-section-title">
        <a class="nav-link px-3 py-1" title="Revocable credentials for the API">
          <i class="fas fa-fw fa-key"></i><span>&nbsp;API tokens</span>
        </a>
      </li>
    </template>

    <!-- Sans identifiant configuré, l'API est ouverte à qui peut l'atteindre.
         Un jeton y ressemblerait à une protection alors que chaque requête
         sans jeton passe toujours. -->
    <div
      v-if="loaded && !guarded"
      class="alert alert-warning d-flex align-items-start gap-2 mt-4 py-2"
    >
      <i class="fas fa-fw fa-triangle-exclamation mt-1"></i>
      <div>
        <div>Nothing guards this wigo.</div>
        <small class="text-body-secondary">
          Its API is open to anyone who can reach it. Set
          <code>Login</code> and <code>Password</code> in the
          <code>[Http]</code> section before minting a token, or the token
          protects nothing.
        </small>
      </div>
    </div>

    <!-- Le secret n'est lisible qu'ici, une seule fois. Rien ne le stocke. -->
    <div
      v-if="created"
      class="alert alert-success mt-4"
      role="alert"
      aria-live="polite"
    >
      <div class="d-flex justify-content-between align-items-start gap-2">
        <div>
          <strong>{{ created.Name }}</strong> created with the
          {{ created.Role }} role.
          <div class="small mt-1">
            This is the only time the secret is shown. Nothing stores it.
          </div>
        </div>
        <button
          type="button"
          class="btn-close"
          aria-label="Dismiss"
          @click="created = null"
        ></button>
      </div>

      <div class="input-group input-group-sm mt-2">
        <input
          class="form-control font-monospace"
          :value="created.Secret"
          readonly
          aria-label="The token secret"
          @focus="$event.target.select()"
        />
        <button
          class="btn btn-outline-secondary"
          type="button"
          @click="copy(created.Secret)"
        >
          <i class="fas fa-fw fa-copy"></i> {{ copied ? "Copied" : "Copy" }}
        </button>
      </div>
    </div>

    <StatusCard v-if="writeAllowed" level="OK" class="mt-4">
      <template #title><strong>Mint a token</strong></template>
      <template #body>
        <form class="row g-2 align-items-end" @submit.prevent="create">
          <div class="col-sm">
            <label class="form-label small mb-1" for="token-name">Name</label>
            <input
              id="token-name"
              v-model="name"
              class="form-control form-control-sm"
              placeholder="e.g. prometheus"
              required
            />
          </div>
          <div class="col-sm-auto">
            <label class="form-label small mb-1" for="token-role">Role</label>
            <select
              id="token-role"
              v-model="role"
              class="form-select form-select-sm"
            >
              <option value="readonly">Read only</option>
              <option value="operator">Operator</option>
            </select>
          </div>
          <div class="col-sm-auto">
            <label class="form-label small mb-1" for="token-expiry">
              Expires
            </label>
            <select
              id="token-expiry"
              v-model="expiry"
              class="form-select form-select-sm"
            >
              <option value="">Never</option>
              <option value="24h">In 1 day</option>
              <option value="168h">In 1 week</option>
              <option value="720h">In 30 days</option>
            </select>
          </div>
          <div class="col-sm-auto">
            <button class="btn btn-sm btn-primary" type="submit">Mint</button>
          </div>
        </form>

        <p class="small text-body-secondary mb-0 mt-2">
          Read only can see everything and change nothing. An operator can also
          disable probes, silence hosts, acknowledge and recheck.
        </p>

        <div v-if="error" class="text-danger small mt-2" role="alert">
          <i class="fas fa-fw fa-triangle-exclamation"></i> {{ error }}
        </div>
      </template>
    </StatusCard>

    <StatusCard level="DISABLED" class="mt-4">
      <template #title><strong>Tokens</strong></template>
      <template #badges>
        <span class="badge text-bg-light">{{ active.length }} active</span>
      </template>
      <template #body>
        <p v-if="loaded && !tokens.length" class="mb-0 text-body-secondary">
          No token yet. The shared credential still works and is an operator.
        </p>

        <table v-else class="table table-bordered table-hover mb-0">
          <thead>
            <tr>
              <th>Name</th>
              <th>Role</th>
              <th>Created</th>
              <th>Last used</th>
              <th>State</th>
              <th v-if="writeAllowed" class="text-end">Revoke</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="token in tokens"
              :key="token.Id"
              :class="{ 'text-body-secondary': !isActive(token) }"
            >
              <td class="align-middle">{{ token.Name }}</td>
              <td class="align-middle">
                <span
                  :class="[
                    'badge',
                    token.Role === 'operator'
                      ? 'text-bg-warning'
                      : 'text-bg-secondary',
                  ]"
                >
                  {{ token.Role }}
                </span>
              </td>
              <td class="align-middle small">
                {{ formatTimestamp(token.CreatedAt) }}
              </td>
              <!-- Un jeton que personne n'utilise est un jeton à révoquer, et
                   c'est la seule façon de le repérer. -->
              <td class="align-middle small">
                {{ token.LastUsed ? formatTimestamp(token.LastUsed) : "never" }}
              </td>
              <td class="align-middle small">{{ describeState(token) }}</td>
              <td v-if="writeAllowed" class="align-middle text-end">
                <button
                  v-if="isActive(token)"
                  class="btn btn-sm btn-outline-danger"
                  type="button"
                  @click="revoke(token)"
                >
                  Revoke
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </template>
    </StatusCard>
  </AppLayout>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from "vue";
import api from "../api/client.js";
import AppLayout from "../components/layout/AppLayout.vue";
import StatusCard from "../components/StatusCard.vue";
import { useRefresh } from "../composables/useRefresh.js";
import { formatTimestamp, humanizeDuration } from "../utils/disable.js";

const tokens = ref([]);
const guarded = ref(true);
const writeAllowed = ref(false);
const loaded = ref(false);
const created = ref(null);
const copied = ref(false);
const error = ref("");

const name = ref("");
const role = ref("readonly");
const expiry = ref("");

const active = computed(() => tokens.value.filter(isActive));

function isActive(token) {
  if (token.RevokedAt) return false;
  return !token.ExpiresAt || token.ExpiresAt * 1000 > Date.now();
}

function describeState(token) {
  if (token.RevokedAt) {
    return `revoked ${humanizeDuration(Date.now() / 1000 - token.RevokedAt)} ago`;
  }
  if (!token.ExpiresAt) return "active";
  if (token.ExpiresAt * 1000 <= Date.now()) return "expired";

  return `expires in ${humanizeDuration(token.ExpiresAt - Date.now() / 1000)}`;
}

async function load() {
  try {
    const answer = await api.getTokens();
    tokens.value = answer.Tokens || [];
    guarded.value = !!answer.Guarded;
    writeAllowed.value = !!answer.WriteActionsAllowed;
    loaded.value = true;
  } catch (requestError) {
    console.error("Error loading the tokens:", requestError);
  }
}

async function create() {
  error.value = "";
  try {
    created.value = await api.createToken(name.value, role.value, expiry.value);
    copied.value = false;
    name.value = "";
    await load();
  } catch (requestError) {
    error.value =
      requestError.response?.data ||
      requestError.message ||
      "The request failed";
  }
}

async function revoke(token) {
  error.value = "";
  try {
    const answer = await api.revokeToken(token.Id);
    tokens.value = answer.Tokens || [];
  } catch (requestError) {
    error.value =
      requestError.response?.data ||
      requestError.message ||
      "The request failed";
  }
}

async function copy(secret) {
  try {
    await navigator.clipboard.writeText(secret);
    copied.value = true;
  } catch {
    // Pas de presse-papier hors contexte sécurisé : le champ est sélectionnable
    copied.value = false;
  }
}

const { startRefresh, stopRefresh, setRefreshInterval, interval } = useRefresh(
  load,
  60,
);

function handleRefreshSettings(seconds) {
  setRefreshInterval(seconds);
}

onMounted(() => {
  load();
  startRefresh();
});

onUnmounted(() => {
  stopRefresh();
});
</script>
