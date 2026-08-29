package wigo

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// Who is asking, and whether they may change anything.
//
// Until now there was one shared basic auth credential for the whole api and
// the whole interface. That was tolerable while everything was read only. It
// stopped being tolerable the moment the api could disable a probe, silence a
// host or acknowledge an alert : anyone handed the dashboard url to look at a
// graph could switch the monitoring off for the entire fleet.
//
// So a caller now has a role. Reading needs the credential, changing anything
// needs an operator. The shared credential stays and stays an operator, because
// an upgrade must not lock an administrator out of their own install -- it is
// the thing you use to mint the first token, and the thing you remove once you
// have.

const (
	// Can read everything. Cannot change anything.
	RoleReadOnly = "readonly"

	// Can also disable probes, silence hosts, acknowledge, recheck, and manage
	// tokens.
	RoleOperator = "operator"

	// Not a role a credential can hold : what an unauthenticated caller is
	// given when this wigo answers nobody without credentials. It exists so
	// "who may read this without identifying themselves" is one setting with
	// three answers rather than a side effect of whether Login is filled in.
	RoleNone = "none"
)

// ApiToken is one revocable credential.
//
// The secret itself is never stored : only its sha256. A token is 32 random
// bytes, not a password, so there is nothing to slow down an attacker who has
// the hash -- guessing it is the problem, and 256 bits of entropy is the answer.
// It also means a stolen database cannot be replayed against the api.
type ApiToken struct {
	Id   int64
	Name string
	Role string

	CreatedAt int64
	ExpiresAt int64 // zero for a token that does not expire
	RevokedAt int64 // zero while it is still good
	LastUsed  int64 // zero until it is first used
}

// Usable reports whether the token may still authenticate anything.
func (token ApiToken) Usable(now int64) bool {
	if token.RevokedAt > 0 {
		return false
	}

	return token.ExpiresAt == 0 || token.ExpiresAt > now
}

const createApiTokensTable = `
    CREATE TABLE IF NOT EXISTS api_tokens (
        id integer not null primary key,
        name text not null,
        hash text not null unique,
        role text not null,
        created_at int,
        expires_at int,
        revoked_at int,
        last_used int
    ) ;
    `

// IsValidRole reports whether a role is one this knows about.
func IsValidRole(role string) bool {
	return role == RoleReadOnly || role == RoleOperator
}

// newTokenSecret produces what the caller will present.
//
// Prefixed so a token found in a log, a shell history or a git diff is
// recognisable for what it is, which is the difference between revoking it and
// not knowing you should.
func newTokenSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("fail to generate a token : %s", err)
	}

	return "wigo_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))

	return hex.EncodeToString(sum[:])
}

// CreateApiToken mints one and returns the secret, which is the only time it
// can ever be read.
func CreateApiToken(name string, role string, expiresAt int64) (ApiToken, string, error) {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return ApiToken{}, "", fmt.Errorf("no database to store a token in")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return ApiToken{}, "", fmt.Errorf("a token needs a name, so it can be recognised and revoked later")
	}
	if !IsValidRole(role) {
		return ApiToken{}, "", fmt.Errorf("unknown role %q, expected %q or %q", role, RoleReadOnly, RoleOperator)
	}
	if expiresAt > 0 && expiresAt <= time.Now().Unix() {
		return ApiToken{}, "", fmt.Errorf("a token cannot expire in the past")
	}

	secret, err := newTokenSecret()
	if err != nil {
		return ApiToken{}, "", err
	}

	token := ApiToken{
		Name:      name,
		Role:      role,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: expiresAt,
	}

	LocalWigo.sqlLiteLock.Lock()
	result, err := LocalWigo.sqlLiteConn.Exec(
		`INSERT INTO api_tokens(name,hash,role,created_at,expires_at,revoked_at,last_used)
		 VALUES(?,?,?,?,?,0,0);`,
		token.Name, hashTokenSecret(secret), token.Role, token.CreatedAt, token.ExpiresAt)
	if err == nil {
		token.Id, _ = result.LastInsertId()
	}
	LocalWigo.sqlLiteLock.Unlock()

	if err != nil {
		return ApiToken{}, "", fmt.Errorf("fail to store the token : %s", err)
	}

	return token, secret, nil
}

// ApiTokens lists them without their secrets, which are not stored.
func ApiTokens() ([]ApiToken, error) {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return nil, fmt.Errorf("no database to read tokens from")
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	rows, err := LocalWigo.sqlLiteConn.Query(
		`SELECT id,name,role,created_at,expires_at,revoked_at,last_used
		 FROM api_tokens ORDER BY created_at;`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]ApiToken, 0)
	for rows.Next() {
		var token ApiToken
		if err := rows.Scan(&token.Id, &token.Name, &token.Role,
			&token.CreatedAt, &token.ExpiresAt, &token.RevokedAt, &token.LastUsed); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// RevokeApiToken stops a token working, keeping the row.
//
// Deleted would be tidier and worse : a revoked token that vanishes takes with
// it the answer to "what was that thing called, and when did we turn it off".
func RevokeApiToken(id int64) error {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return fmt.Errorf("no database to revoke a token in")
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	result, err := LocalWigo.sqlLiteConn.Exec(
		`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at = 0;`,
		time.Now().Unix(), id)
	if err != nil {
		return err
	}

	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("no token with id %d is still active", id)
	}

	return nil
}

// authenticateToken resolves a presented secret to a role.
func authenticateToken(secret string) (ApiToken, bool) {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil || secret == "" {
		return ApiToken{}, false
	}

	LocalWigo.sqlLiteLock.Lock()

	var token ApiToken
	err := LocalWigo.sqlLiteConn.QueryRow(
		`SELECT id,name,role,created_at,expires_at,revoked_at,last_used
		 FROM api_tokens WHERE hash = ?;`, hashTokenSecret(secret)).
		Scan(&token.Id, &token.Name, &token.Role,
			&token.CreatedAt, &token.ExpiresAt, &token.RevokedAt, &token.LastUsed)

	LocalWigo.sqlLiteLock.Unlock()

	if err != nil || !token.Usable(time.Now().Unix()) {
		return ApiToken{}, false
	}

	noteTokenUsed(token.Id)

	return token, true
}

// noteTokenUsed records when a token last worked, which is what makes it
// possible to find the ones nobody uses any more and revoke them.
func noteTokenUsed(id int64) {
	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(
		`UPDATE api_tokens SET last_used = ? WHERE id = ?;`, time.Now().Unix(), id); err != nil {
		log.Printf("Unable to record the use of token %d : %s", id, err)
	}
}

// Minting the first token.
//
// Everything else about tokens goes through the api, which needs a credential,
// which is the thing a first token is for. That circle has to be broken
// somewhere, and the only place that owes nobody an authentication is the
// machine's own filesystem : whoever can read the database can already read
// every secret wigo holds.
//
// So this opens the database and nothing else. Not the probes directory, not
// the log file, not the push server -- none of which has anything to do with
// minting a token, and any of which failing would be a reason not to be able
// to, which is exactly the corner this exists to get out of.

// OpenForTokens prepares just enough of a wigo to read and write its tokens.
func OpenForTokens(configFile string) error {
	config := NewConfig(configFile)

	wigo := new(Wigo)
	wigo.config = config
	wigo.locker = new(sync.RWMutex)
	wigo.sqlLiteLock = new(sync.Mutex)

	connection, err := sql.Open("sqlite", config.Global.Database)
	if err != nil {
		return fmt.Errorf("cannot open the database %s : %s", config.Global.Database, err)
	}

	// Created if missing : a wigo that has never run has no tables, and asking
	// somebody to start it first would be asking them to start the thing they
	// cannot authenticate against.
	if _, err := connection.Exec(createApiTokensTable); err != nil {
		return fmt.Errorf("cannot prepare the tokens table in %s : %s", config.Global.Database, err)
	}

	wigo.sqlLiteConn = connection
	LocalWigo = wigo

	return nil
}

// ParseTokenExpiry reads how long a token is good for. Empty means for ever,
// which is right for the one a scraper uses and wrong for the one you hand
// somebody for an afternoon.
func ParseTokenExpiry(value string) (int64, error) {
	return parseTokenExpiry(value)
}
