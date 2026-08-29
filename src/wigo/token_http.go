package wigo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// TokenList is what GET /api/tokens answers.
//
// Never the secrets : they are not stored, only their hash. A token is readable
// exactly once, in the answer that created it, which is the whole point of not
// being able to lose control of one that leaked.
type TokenList struct {
	// Whether the caller may mint or revoke any of these.
	WriteActionsAllowed bool

	// Whether anything guards this wigo at all. False means no credential is
	// configured, so a token would be pointless : the api is already open.
	Guarded bool

	Tokens []ApiToken
}

// HttpTokensHandler lists the tokens, without their secrets.
func HttpTokensHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	tokens, err := ApiTokens()
	if err != nil {
		return 500, fmt.Sprintf("Fail to read the tokens : %s", err)
	}

	_, _, allowed := httpWriteActionsAllowed(r)

	body, err := json.Marshal(TokenList{
		WriteActionsAllowed: allowed,
		Guarded:             GetLocalWigo().GetConfig().Http.Login != "",
		Tokens:              tokens,
	})
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the tokens : %s", err)
	}

	return 200, string(body)
}

// HttpTokenCreateHandler mints one and answers the secret, once.
func HttpTokenCreateHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	if status, message, allowed := httpWriteActionsAllowed(r); !allowed {
		return status, message
	}

	// A token on an unguarded wigo is a lock on an open door : it would look
	// like protection while every request without one still walks through.
	if GetLocalWigo().GetConfig().Http.Login == "" {
		return 400, "This wigo has no credential configured, so its api is open to anyone who can reach it. " +
			"Set Login and Password in the [Http] section first, or a token protects nothing."
	}

	role := r.URL.Query().Get("role")
	if role == "" {
		role = RoleReadOnly
	}

	expiresAt, err := parseTokenExpiry(r.URL.Query().Get("for"))
	if err != nil {
		return 400, err.Error()
	}

	token, secret, err := CreateApiToken(r.URL.Query().Get("name"), role, expiresAt)
	if err != nil {
		return 400, err.Error()
	}

	GetLocalWigo().AddLog(nil, INFO, fmt.Sprintf("Api token %q created with the %s role by %s",
		token.Name, token.Role, describeAuthor(CallerOf(r).Name)))

	body, err := json.Marshal(struct {
		ApiToken

		// Shown once and never again. Nothing stores it.
		Secret string
	}{ApiToken: token, Secret: secret})
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the token : %s", err)
	}

	return 200, string(body)
}

// HttpTokenRevokeHandler stops a token working.
func HttpTokenRevokeHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	if status, message, allowed := httpWriteActionsAllowed(r); !allowed {
		return status, message
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 400, fmt.Sprintf("invalid token id %q", r.PathValue("id"))
	}

	if err := RevokeApiToken(id); err != nil {
		return 400, err.Error()
	}

	GetLocalWigo().AddLog(nil, INFO, fmt.Sprintf("Api token %d revoked by %s",
		id, describeAuthor(CallerOf(r).Name)))

	return HttpTokensHandler(w, r)
}

// HttpWhoamiHandler says what the caller may do.
//
// The interface needs it to know whether to offer a control at all : a button
// that always answers 403 is worse than no button.
func HttpWhoamiHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	caller := CallerOf(r)

	http := GetLocalWigo().GetConfig().Http

	body, err := json.Marshal(struct {
		Name                string
		Role                string
		WriteActionsAllowed bool

		// Whether there is a credential to sign in with at all. Offering the
		// interface a sign-in button on a host that has no Login configured
		// would be offering a door with no key behind it.
		CanSignIn bool
	}{
		Name:                caller.Name,
		Role:                caller.Role,
		WriteActionsAllowed: http.AllowWriteActions && caller.May(RoleOperator),
		CanSignIn:           http.Login != "" && http.Password != "",
	})
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the caller : %s", err)
	}

	return 200, string(body)
}

// parseTokenExpiry reads how long a token is good for. Empty means for ever,
// which is right for the one a scraper uses and wrong for the one you hand
// somebody for an afternoon.
func parseTokenExpiry(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q, expected something like 24h or 720h", value)
	}

	if duration < time.Minute {
		return 0, fmt.Errorf("a token that expires in less than a minute is not worth minting")
	}

	return time.Now().Add(duration).Unix(), nil
}
