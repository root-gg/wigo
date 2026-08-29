import { ref, onMounted, onUnmounted } from "vue";

/**
 * Écoute ce qui se passe côté serveur, et redemande quand quelque chose bouge.
 *
 * Ce qui circule est volontairement maigre : *quoi* a changé, pas en quoi. Le
 * navigateur va rechercher, ce qui garde une seule sérialisation de l'état au
 * lieu de deux et rend un événement manqué sans conséquence.
 *
 * Le rafraîchissement périodique n'est pas remplacé, il reste le filet : un
 * flux peut mourir sans bruit — un proxy, une mise en veille — et une page qui
 * ne se rafraîchit plus tout en paraissant à jour est pire que pas de flux du
 * tout.
 */
export function useLiveEvents(onChange, { enabled = true } = {}) {
  const connected = ref(false);
  let source = null;
  let reconnectTimer = null;

  // Une rafale de changements ne doit pas déclencher une rafale de requêtes :
  // dix sondes qui basculent ensemble, c'est un seul état à relire.
  let pending = null;
  function scheduleRefresh() {
    if (pending) return;
    pending = setTimeout(() => {
      pending = null;
      onChange();
    }, 250);
  }

  function connect() {
    if (!enabled || typeof EventSource === "undefined") return;

    close();

    try {
      source = new EventSource("api/events");
    } catch {
      // Rien de plus à faire : le rafraîchissement périodique prend le relais
      return;
    }

    source.onopen = () => {
      connected.value = true;
    };

    for (const name of ["probe", "host", "schedule", "message"]) {
      source.addEventListener(name, scheduleRefresh);
    }

    source.onerror = () => {
      connected.value = false;

      // EventSource se reconnecte tout seul, sauf quand la connexion est
      // fermée pour de bon. Là il faut la rouvrir soi-même.
      if (source && source.readyState === EventSource.CLOSED) {
        close();
        reconnectTimer = setTimeout(connect, 10000);
      }
    };
  }

  function close() {
    if (source) {
      source.close();
      source = null;
    }
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    connected.value = false;
  }

  onMounted(connect);
  onUnmounted(() => {
    close();
    if (pending) clearTimeout(pending);
  });

  return { connected, close };
}
