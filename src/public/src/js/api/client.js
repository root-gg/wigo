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
export const api = {
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
  async getHosts() {
    const response = await apiClient.get("/hosts");
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
   * Récupère l'ordonnancement des probes du host qui sert cette interface :
   * intervalle de chacune, y compris celles qui sont désactivées et n'ont donc
   * aucun résultat.
   * @returns {Promise<{Hostname: string, WriteActionsAllowed: boolean, Probes: Array}>}
   */
  async getProbesSchedule() {
    const response = await apiClient.get("/probes");
    return response.data;
  },

  /**
   * Désactive une probe : elle n'est plus exécutée du tout
   * @param {string} probeName - Nom de la probe
   * @returns {Promise<Object>} L'ordonnancement mis à jour
   */
  async disableProbe(probeName) {
    const response = await apiClient.post(
      `/probes/${encodeURIComponent(probeName)}/disable`,
    );
    return response.data;
  },

  /**
   * Change l'intervalle d'une probe, ce qui la réactive si elle était désactivée
   * @param {string} probeName - Nom de la probe
   * @param {number} seconds - Intervalle en secondes
   * @returns {Promise<Object>} L'ordonnancement mis à jour
   */
  async setProbeInterval(probeName, seconds) {
    const response = await apiClient.post(
      `/probes/${encodeURIComponent(probeName)}/interval`,
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
