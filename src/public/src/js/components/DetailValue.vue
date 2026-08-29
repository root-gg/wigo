<template>
  <!-- Absent et vide ne se ressemblent pas : une colonne qu'une ligne ne
       renseigne pas doit se voir comme telle, pas comme une chaîne vide. -->
  <span v-if="value === null || value === undefined" class="text-body-secondary"
    >&mdash;</span
  >

  <span v-else-if="typeof value === 'boolean'">
    <i
      :class="[
        'fas',
        'fa-fw',
        value ? 'fa-check text-success' : 'fa-xmark text-danger',
      ]"
    ></i>
  </span>

  <!-- Une liste : une par ligne. Jointes par des virgules, deux avertissements
       longs se lisent comme un seul. -->
  <ul v-else-if="Array.isArray(value)" class="list-unstyled mb-0">
    <li v-for="(one, index) in value" :key="index">{{ one }}</li>
  </ul>

  <!-- Une mesure ne se coupe pas en deux lignes : « 0.00 » au-dessus de
       « req/s » se lit comme deux valeurs. Le texte, lui, garde le droit
       de passer à la ligne, sinon un avertissement long rendrait le
       tableau plus large que l'écran. -->
  <span v-else :class="looksNumeric ? 'font-monospace text-nowrap' : ''">{{
    value
  }}</span>
</template>

<script setup>
import { computed } from "vue";

const props = defineProps({
  value: { type: null, default: null },
});

/**
 * Les chiffres alignés se comparent, le texte proportionnel non : vingt-trois
 * lignes de « 0.00 kB/s » ne veulent rien dire tant que les virgules ne sont
 * pas les unes sous les autres.
 */
const looksNumeric = computed(() => {
  if (typeof props.value === "number") return true;

  return (
    typeof props.value === "string" &&
    /^[\d\s.,+-]+\s*\S{0,8}$/.test(props.value)
  );
});
</script>
