<template>
  <!-- Ce que le host est, avant ce qui lui arrive. Chaque label est un lien
       vers la flotte filtrée sur lui : le geste attendu en lisant "env=prod"
       est de vouloir voir le reste de la prod. -->
  <div v-if="labels.length" class="d-flex flex-wrap align-items-center gap-1">
    <router-link
      v-for="label in labels"
      :key="label.key"
      class="badge text-bg-secondary text-decoration-none host-label"
      :to="{ path: '/', query: { labels: `${label.key}=${label.value}` } }"
      :title="`Show every host with ${label.key}=${label.value}`"
    >
      <span class="opacity-75">{{ label.key }}</span
      >=<span class="fw-semibold">{{ label.value }}</span>
    </router-link>
  </div>
</template>

<script setup>
import { computed } from "vue";

const props = defineProps({
  /** Le Host tel que l'api le rend : Group et Labels */
  host: { type: Object, default: null },
});

/**
 * Les mêmes labels que ceux sur lesquels le serveur filtre, dérivation du
 * groupe comprise.
 *
 * Le groupe est ajouté ici plutôt que lu dans Labels, exactement comme le fait
 * `LabelsOf` côté serveur : les deux doivent donner la même chose, sinon un
 * label affiché ici ne ramènerait rien une fois cliqué.
 */
const labels = computed(() => {
  const host = props.host;
  if (!host) return [];

  const collected = { ...(host.Labels || {}) };
  if (host.Group) collected.group = host.Group;

  return Object.keys(collected)
    .sort()
    .map((key) => ({ key, value: collected[key] }));
});
</script>

<style scoped>
/* Un lien reste un lien : il doit se voir survolable sans changer de couleur,
   qui ici porte déjà un sens. */
.host-label:hover {
  filter: brightness(1.25);
}
</style>
