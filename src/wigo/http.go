package wigo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/codegangsta/martini"
)

func HttpRemotesHandler(params martini.Params) (int, string) {

	hostname := params["hostname"]

	if hostname != "" {
		remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
		if remoteWigo != nil {
			json, err := remoteWigo.ToJsonString()
			if err != nil {
				return 500, "Failed to encode remote wigo"
			} else {
				return 200, json
			}
		} else {
			return 404, ""
		}
	}

	// Return remotes list
	list := GetLocalWigo().ListRemoteWigosNames()
	json, err := json.Marshal(list)
	if err != nil {
		return 500, ""
	} else {
		return 200, string(json)
	}
}

func HttpRemotesProbesHandler(params martini.Params) (int, string) {

	hostname := params["hostname"]
	probeName := params["probe"]

	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	// Get remote wigo
	remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remoteWigo == nil {
		return 404, "Remote wigo " + hostname + " not found"
	}

	// Get probe or probes
	if probeName != "" {
		if tmp, ok := remoteWigo.LocalHost.Probes.Get(probeName); ok {
			probe := tmp.(*ProbeResult)

			json, err := json.Marshal(probe)
			if err != nil {
				return 500, ""
			} else {
				return 200, string(json)
			}
		}
	} else {
		json, err := json.Marshal(remoteWigo.ListProbes())
		if err != nil {
			return 500, ""
		} else {
			return 200, string(json)
		}
	}

	return 200, ""
}

func HttpRemotesStatusHandler(params martini.Params) (int, string) {

	hostname := params["hostname"]

	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	// Get remote wigo
	remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remoteWigo == nil {
		return 404, "Remote wigo " + hostname + " not found"
	}

	return 200, strconv.Itoa(remoteWigo.GlobalStatus)
}

func HttpRemotesProbesStatusHandler(params martini.Params) (int, string) {

	hostname := params["hostname"]
	probeName := params["probe"]

	if hostname == "" {
		return 404, "No wigo name set in url"
	}
	if probeName == "" {
		return 404, "No probe name set in url"
	}

	// Get remote wigo
	remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remoteWigo == nil {
		return 404, "Remote wigo " + hostname + " not found"
	}

	// Get probe
	if tmp, ok := remoteWigo.LocalHost.Probes.Get(probeName); ok {
		probe := tmp.(*ProbeResult)
		return 200, strconv.Itoa(probe.Status)
	} else {
		return 404, "Probe " + probeName + " not found in remote wigo " + hostname
	}

}

func HttpLogsHandler(params martini.Params, r *http.Request) (int, string) {

	//Parse url
	u, err := url.Parse(r.URL.String())
	if err != nil {
		return 500, fmt.Sprintf("%s", err)
	}
	pq, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return 500, fmt.Sprintf("%s", err)
	}

	// Get params
	hostname := ""
	if len(pq["hostname"]) > 0 {
		hostname = pq["hostname"][0]
	}
	probeName := ""
	if len(pq["probe"]) > 0 {
		probeName = pq["probe"][0]
	}
	group := ""
	if len(pq["group"]) > 0 {
		group = pq["group"][0]
	}

	// Index && Offset
	limit := 100
	if len(pq["limit"]) > 0 {
		if iInt, err := strconv.Atoi(pq["limit"][0]); err == nil {
			limit = iInt
		}
	}
	offset := 0
	if len(pq["offset"]) > 0 {
		if oInt, err := strconv.Atoi(pq["offset"][0]); err == nil {
			offset = oInt
		}
	}

	// Test hostname if present
	var remoteWigo *Wigo
	if hostname != "" {
		remoteWigo = GetLocalWigo().FindRemoteWigoByHostname(hostname)
		if remoteWigo == nil {
			return 404, "Remote wigo " + hostname + " not found"
		}
	}

	// Test probe
	if probeName != "" {
		if hostname != "" {
			// Get probe
			if _, ok := remoteWigo.LocalHost.Probes.Get(probeName); ok {
			} else {
				return 404, "Probe " + probeName + " not found in remote wigo " + hostname
			}
		}
	}

	// Get logs
	logs := LocalWigo.SearchLogs(probeName, hostname, group, uint64(limit), uint64(offset))

	// Json
	json, err := json.Marshal(logs)
	if err != nil {
		return 500, ""
	}

	return 200, string(json)
}

func HttpGroupsHandler(params martini.Params) (int, string) {

	group := params["group"]

	result := make(map[string]interface{})
	result["Name"] = group

	if group != "" {
		gs, status := GetLocalWigo().GroupSummary(group)
		if gs != nil {

			result["Status"] = status
			result["Hosts"] = gs

			json, err := json.Marshal(result)
			if err != nil {
				return 500, fmt.Sprintf("Fail to encode summary : %s", err)
			} else {
				return 200, string(json)
			}
		} else {
			return 404, ""
		}
	}

	// Return remotes list
	list := GetLocalWigo().ListGroupsNames()
	json, err := json.Marshal(list)
	if err != nil {
		return 500, ""
	} else {
		return 200, string(json)
	}
}

func HttpLogsIndexesHandler(params martini.Params) (int, string) {

	result := make(map[string][]string)
	result["probes"] = make([]string, 0)
	result["hosts"] = make([]string, 0)
	result["groups"] = make([]string, 0)

	// Queries
	qP := "SELECT DISTINCT(probe) FROM logs"
	qH := "SELECT DISTINCT(host) FROM logs"
	qG := "SELECT DISTINCT(grp) FROM logs"

	// Probes
	if rowsProbes, err := LocalWigo.sqlLiteConn.Query(qP); err == nil {
		for rowsProbes.Next() {
			var p string
			if err := rowsProbes.Scan(&p); err == nil {
				result["probes"] = append(result["probes"], p)
			}
		}
	}

	// Hosts
	if rowsHosts, err := LocalWigo.sqlLiteConn.Query(qH); err == nil {
		for rowsHosts.Next() {
			var h string
			if err := rowsHosts.Scan(&h); err == nil {
				result["hosts"] = append(result["hosts"], h)
			}
		}
	}

	// Groups
	if rowsGroup, err := LocalWigo.sqlLiteConn.Query(qG); err == nil {
		for rowsGroup.Next() {
			var g string
			if err := rowsGroup.Scan(&g); err == nil {
				result["groups"] = append(result["groups"], g)
			}
		}
	}

	// Return remotes list
	json, err := json.Marshal(result)
	if err != nil {
		return 500, fmt.Sprintf("Error while encoding to json : %s", err)
	} else {
		return 200, string(json)
	}
}

func HttpAuthorityListHandler(params martini.Params) (int, string) {

	result := make(map[string]map[string]string)

	if LocalWigo.push == nil {
		return 500, "Push server is not started"
	}

	result["waiting"] = LocalWigo.push.authority.Waiting
	result["allowed"] = LocalWigo.push.authority.Allowed

	// Return remotes list
	json, err := json.Marshal(result)
	if err != nil {
		return 500, fmt.Sprintf("Error while encoding to json : %s", err)
	} else {
		return 200, string(json)
	}
}

func HttpAuthorityAllowHandler(params martini.Params) (int, string) {

	uuid := params["uuid"]

	if LocalWigo.push == nil {
		return 500, "Push server is not started"
	}

	err := LocalWigo.push.authority.AllowClient(uuid)

	if err != nil {
		return 500, err.Error()
	}

	return 200, "OK"
}

func HttpAuthorityRevokeHandler(params martini.Params) (int, string) {

	uuid := params["uuid"]

	if LocalWigo.push == nil {
		return 500, "Push server is not started"
	}

	err := LocalWigo.push.authority.RevokeClient(uuid)

	if err != nil {
		return 500, err.Error()
	}

	return 200, "OK"
}
