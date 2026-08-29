import { ref, onUnmounted } from "vue";

/**
 * Composable pour gérer le rafraîchissement automatique
 * @param {Function} callback - Fonction à appeler lors du rafraîchissement
 * @param {number} defaultInterval - Intervalle par défaut en secondes
 * @returns {Object} Objet avec startRefresh, stopRefresh, setInterval, interval
 */
export function useRefresh(callback, defaultInterval = 60) {
  const interval = ref(defaultInterval);
  const timeoutId = ref(null);
  const running = ref(false);

  // Incrémenté à chaque start/stop : un cycle démarré avant un changement
  // d'intervalle se reconnaît périmé et ne réarme pas de tick.
  let generation = 0;

  function startRefresh() {
    stopRefresh();
    if (interval.value <= 0) return;

    running.value = true;
    const cycle = ++generation;

    // Le prochain tick n'est armé qu'une fois le rafraîchissement terminé,
    // sinon un chargement plus lent que l'intervalle empile les requêtes.
    const tick = async () => {
      if (cycle !== generation) return;
      try {
        await callback();
      } finally {
        if (cycle === generation) {
          timeoutId.value = setTimeout(tick, interval.value * 1000);
        }
      }
    };

    timeoutId.value = setTimeout(tick, interval.value * 1000);
  }

  function stopRefresh() {
    generation++;
    running.value = false;
    if (timeoutId.value) {
      clearTimeout(timeoutId.value);
      timeoutId.value = null;
    }
  }

  function setRefreshInterval(newInterval) {
    const wasRunning = running.value;
    interval.value = newInterval;
    if (wasRunning) {
      startRefresh();
    }
  }

  onUnmounted(() => {
    stopRefresh();
  });

  return {
    interval,
    startRefresh,
    stopRefresh,
    setRefreshInterval,
  };
}
