import axios from "axios";

const API_BASE_URL = "/api";

// Instance Axios configurée
const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

/**
 * Service API pour remplacer Restangular
 */
export /** Les paramètres vides ne partent pas : l'API distingue absent de vide */
async function postSuppression(path, params) {
  const query = {};
  for (const [key, value] of Object.entries(params)) {
    if (value) query[key] = value;
  }

  const response = await apiClient.post(path, null, { params: query });
  return response.data;
}

const api = {
  /**
   * Récupère la liste des groupes
   * @returns {Promise<Array<string>>}
   */
  async getGroups() {
    const response = await apiClient.get("/groups");
    return response.data;
  },

  /**
   * Récupère les détails d'un groupe
   * @param {string} groupName - Nom du groupe
   * @returns {Promise<Object>}
   */
  async getGroup(groupName) {
    const response = await apiClient.get(`/groups/${groupName}`);
    return response.data;
  },

  /**
   * Récupère la liste des hosts
   * @returns {Promise<Array>}
   */
  async getHosts(labels) {
    const response = await apiClient.get("/hosts", {
      params: labels ? { labels } : undefined,
    });
    return response.data;
  },

  /**
   * Les labels en usage dans la flotte, avec le nombre de hosts par valeur.
   * @returns {Promise<Object>}
   */
  async getLabels() {
    const response = await apiClient.get("/labels");
    return response.data;
  },

  /**
   * Récupère les détails d'un host
   * @param {string} hostname - Nom du host
   * @returns {Promise<Object>}
   */
  async getHost(hostname) {
    const response = await apiClient.get(`/hosts/${hostname}`);
    return response.data;
  },

  /**
   * Récupère le statut d'un host
   * @param {string} hostname - Nom du host
   * @returns {Promise<number>}
   */
  async getHostStatus(hostname) {
    const response = await apiClient.get(`/hosts/${hostname}/status`);
    return parseInt(response.data);
  },

  /**
   * Récupère les probes d'un host
   * @param {string} hostname - Nom du host
   * @returns {Promise<Array>}
   */
  async getHostProbes(hostname) {
    const response = await apiClient.get(`/hosts/${hostname}/probes`);
    return response.data;
  },

  /**
   * Récupère les détails d'une probe
   * @param {string} hostname - Nom du host
   * @param {string} probeName - Nom de la probe
   * @returns {Promise<Object>}
   */
  async getProbe(hostname, probeName) {
    const response = await apiClient.get(
      `/hosts/${hostname}/probes/${probeName}`,
    );
    return response.data;
  },

  /**
   * Récupère le statut d'une probe
   * @param {string} hostname - Nom du host
   * @param {string} probeName - Nom de la probe
   * @returns {Promise<number>}
   */
  async getProbeStatus(hostname, probeName) {
    const response = await apiClient.get(
      `/hosts/${hostname}/probes/${probeName}/status`,
    );
    return parseInt(response.data);
  },

  /**
   * Récupère les logs
   * @param {Object} params - Paramètres de requête (offset, limit, group, host, probe)
   * @returns {Promise<Array>}
   */
  async getLogs(params = {}) {
    const response = await apiClient.get("/logs", { params });
    return response.data;
  },

  /**
   * Récupère les logs d'un groupe
   * @param {string} groupName - Nom du groupe
   * @param {Object} params - Paramètres de requête (offset, limit)
   * @returns {Promise<Array>}
   */
  async getGroupLogs(groupName, params = {}) {
    const response = await apiClient.get(`/groups/${groupName}/logs`, {
      params,
    });
    return response.data;
  },

  /**
   * Récupère les logs d'un host
   * @param {string} hostname - Nom du host
   * @param {Object} params - Paramètres de requête (offset, limit)
   * @returns {Promise<Array>}
   */
  async getHostLogs(hostname, params = {}) {
    const response = await apiClient.get(`/hosts/${hostname}/logs`, { params });
    return response.data;
  },

  /**
   * Récupère les logs d'une probe
   * @param {string} hostname - Nom du host
   * @param {string} probeName - Nom de la probe
   * @param {Object} params - Paramètres de requête (offset, limit)
   * @returns {Promise<Array>}
   */
  async getProbeLogs(hostname, probeName, params = {}) {
    const response = await apiClient.get(
      `/hosts/${hostname}/probes/${probeName}/logs`,
      { params },
    );
    return response.data;
  },

  /**
   * Récupère les logs d'une probe (route alternative)
   * @param {string} probeName - Nom de la probe
   * @param {Object} params - Paramètres de requête (offset, limit)
   * @returns {Promise<Array>}
   */
  async getProbeLogsByName(probeName, params = {}) {
    const response = await apiClient.get(`/probes/${probeName}/logs`, {
      params,
    });
    return response.data;
  },

  /**
   * Récupère les index des logs
   * @returns {Promise<Object>}
   */
  async getLogIndexes() {
    const response = await apiClient.get("/logs/indexes");
    return response.data;
  },

  /**
   * Récupère le statut global
   * @returns {Promise<number>}
   */
  async getStatus() {
    const response = await apiClient.get("/status");
    return parseInt(response.data);
  },

  /**
   * Récupère les données complètes
   * @returns {Promise<Object>}
   */
  async getAll() {
    const response = await apiClient.get("/");
    return response.data;
  },

  /**
   * Récupère l'ordonnancement des probes d'un host : intervalle de chacune, y
   * compris celles qui sont désactivées et n'ont donc aucun résultat. Le master
   * relaie la demande au host visé, donc l'appel a la même forme qu'il s'agisse
   * de lui-même ou d'un remote.
   * @param {string} hostname - Nom du host
   * @returns {Promise<{Hostname: string, WriteActionsAllowed: boolean, Probes: Array}>}
   */
  async getHostProbesSchedule(hostname) {
    const response = await apiClient.get(
      `/hosts/${encodeURIComponent(hostname)}/schedule`,
    );
    return response.data;
  },

  /**
   * Désactive une probe : elle n'est plus exécutée du tout
   * @param {string} hostname - Nom du host
   * @param {string} probeName - Nom de la probe
   * @returns {Promise<Object>} L'ordonnancement mis à jour
   */
  /**
   * L'historique d'une sonde. Les points sont regroupés côté serveur : une
   * semaine à un point par minute, c'est dix mille points par série.
   */
  async getProbeMetrics(hostname, probeName, since, until, points) {
    const params = { since };
    if (until) params.until = until;
    if (points) params.points = points;

    const response = await apiClient.get(
      `/hosts/${encodeURIComponent(hostname)}/probes/${encodeURIComponent(probeName)}/metrics`,
      { params },
    );
    return response.data;
  },

  /**
   * Qui appelle, et ce qu'il a le droit de faire.
   *
   * Séparé des réponses qui portent déjà WriteActionsAllowed : celles-là
   * parlent d'un host, celle-ci parle de la session, et la barre du haut a
   * besoin de la seconde avant de savoir quel host on regarde.
   */
  async getWhoami() {
    const response = await apiClient.get("/whoami");
    return response.data;
  },

  /**
   * L'historique des changements de statut. Toujours servi par ce wigo-ci et
   * jamais relayé : un maître a vu de son remote des choses que celui-ci ne
   * peut pas avoir enregistrées sur lui-même, tomber en étant la première.
   */
  async getHostTimeline(hostname, probeName, since, until) {
    const params = { since };
    if (probeName) params.probe = probeName;
    if (until) params.until = until;

    const response = await apiClient.get(
      `/hosts/${encodeURIComponent(hostname)}/timeline`,
      { params },
    );
    return response.data;
  },

  /** Les jetons d'API, jamais leurs secrets : ils ne sont pas stockés */
  async getTokens() {
    const response = await apiClient.get("/tokens");
    return response.data;
  },

  /**
   * Frappe un jeton. Le secret n'est lisible que dans cette réponse, une seule
   * fois : rien ne le stocke.
   */
  async createToken(name, role, duration) {
    const params = { name, role };
    if (duration) params.for = duration;

    const response = await apiClient.post("/tokens", null, { params });
    return response.data;
  },

  async revokeToken(id) {
    const response = await apiClient.post(
      `/tokens/${encodeURIComponent(id)}/revoke`,
    );
    return response.data;
  },

  /** Ce qui est actuellement mis en sourdine, tous scopes confondus */
  async getSuppressions() {
    const response = await apiClient.get("/suppressions");
    return response.data;
  },

  /**
   * Acquitte l'état courant d'un host, ou d'une de ses probes.
   *
   * L'ack ne porte que sur l'état au moment où il est pris : une aggravation
   * repasse au travers, et un retour à la normale l'efface.
   */
  async ackHost(hostname, probeName, reason) {
    return postSuppression(`/hosts/${encodeURIComponent(hostname)}/ack`, {
      probe: probeName,
      reason,
    });
  },

  /** Fait taire un host, ou une de ses probes, pendant une durée donnée */
  async silenceHost(hostname, probeName, duration, reason) {
    return postSuppression(`/hosts/${encodeURIComponent(hostname)}/silence`, {
      probe: probeName,
      for: duration,
      reason,
    });
  },

  /** Fait taire tout un groupe */
  async silenceGroup(groupName, duration, reason) {
    return postSuppression(`/groups/${encodeURIComponent(groupName)}/silence`, {
      for: duration,
      reason,
    });
  },

  /** Remet les notifications */
  async unsuppressHost(hostname, probeName) {
    return postSuppression(
      `/hosts/${encodeURIComponent(hostname)}/unsuppress`,
      {
        probe: probeName,
      },
    );
  },

  async unsuppressGroup(groupName) {
    return postSuppression(
      `/groups/${encodeURIComponent(groupName)}/unsuppress`,
      {},
    );
  },

  /**
   * Relance une probe immédiatement, hors de son cycle. Répond le résultat
   * frais, ou une phrase quand le host pousse et que l'ordre a été mis en file.
   */
  async runHostProbe(hostname, probeName) {
    const response = await apiClient.post(
      `/hosts/${encodeURIComponent(hostname)}/probes/${encodeURIComponent(probeName)}/run`,
    );
    return response.data;
  },

  /**
   * Désactive une probe. La raison et la durée sont facultatives : sans durée
   * la probe reste désactivée jusqu'à ce que quelqu'un la rallume.
   *
   * @param {string} [reason] pourquoi, tel quel
   * @param {string} [duration] une durée Go, "1h" ou "168h", vide pour aucune
   */
  async disableHostProbe(hostname, probeName, reason, duration) {
    const params = {};
    if (reason) params.reason = reason;
    if (duration) params.for = duration;

    const response = await apiClient.post(
      `/hosts/${encodeURIComponent(hostname)}/probes/${encodeURIComponent(probeName)}/disable`,
      null,
      { params },
    );
    return response.data;
  },

  /**
   * Change l'intervalle d'une probe, ce qui la réactive si elle était désactivée
   * @param {string} hostname - Nom du host
   * @param {string} probeName - Nom de la probe
   * @param {number} seconds - Intervalle en secondes
   * @returns {Promise<Object>} L'ordonnancement mis à jour
   */
  async setHostProbeInterval(hostname, probeName, seconds) {
    const response = await apiClient.post(
      `/hosts/${encodeURIComponent(hostname)}/probes/${encodeURIComponent(probeName)}/interval`,
      null,
      { params: { seconds } },
    );
    return response.data;
  },

  /**
   * Récupère la liste des hosts en attente/autorisation
   * @returns {Promise<Object>}
   */
  async getAuthorityHosts() {
    const response = await apiClient.get("/authority/hosts");
    return response.data;
  },

  /**
   * Autorise un host
   * @param {string} uuid - UUID du host
   * @returns {Promise<void>}
   */
  async allowHost(uuid) {
    await apiClient.post(`/authority/hosts/${uuid}/allow`);
  },

  /**
   * Révoque l'autorisation d'un host
   * @param {string} uuid - UUID du host
   * @returns {Promise<void>}
   */
  async revokeHost(uuid) {
    await apiClient.post(`/authority/hosts/${uuid}/revoke`);
  },
};

export default api;
