/**
 * Reads the minimum account identity needed from Codex's auth.json.
 *
 * AccountID supports matching borrowed rate-limit windows. AccountEmail
 * decodes only the local id_token payload when the user explicitly opts into
 * displaying it. Neither function logs, caches, or persists credentials.
 */
package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AccountID returns the ChatGPT account id recorded in $CODEX_HOME/auth.json,
// or "" when Codex has never signed in on this machine — in which case there
// is no local identity to check a borrowed observation against.
func AccountID() string {
	return AccountIDIn(codexHome())
}

// AccountIDIn reads auth.json under the given Codex home.
func AccountIDIn(home string) string {
	if home == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return ""
	}
	var parsed struct {
		Tokens *struct {
			AccountID *string `json:"account_id"`
		} `json:"tokens"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.Tokens == nil || parsed.Tokens.AccountID == nil {
		return ""
	}
	return strings.TrimSpace(*parsed.Tokens.AccountID)
}

// AccountEmail returns the email claim from the id_token in the active Codex
// home. Callers must gate this behind the explicit UI preference.
func AccountEmail() string {
	return AccountEmailIn(codexHome())
}

// AccountEmailIn reads auth.json under home and returns only the email string
// decoded from tokens.id_token. JWT signatures are deliberately not checked:
// this is local display metadata, not an authentication decision.
func AccountEmailIn(home string) string {
	if home == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return ""
	}
	var parsed struct {
		Tokens *struct {
			IDToken *string `json:"id_token"`
		} `json:"tokens"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.Tokens == nil || parsed.Tokens.IDToken == nil {
		return ""
	}
	return emailFromIDToken(*parsed.Tokens.IDToken)
}

func emailFromIDToken(token string) string {
	header, rest, ok := strings.Cut(token, ".")
	if !ok || header == "" {
		return ""
	}
	payloadSegment, signature, ok := strings.Cut(rest, ".")
	if !ok || payloadSegment == "" || signature == "" || strings.Contains(signature, ".") {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadSegment)
	if err != nil {
		return ""
	}
	var claims struct {
		Email   json.RawMessage `json:"email"`
		Profile json.RawMessage `json:"https://api.openai.com/profile"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	if email := emailClaim(claims.Email); email != "" {
		return email
	}
	var profile struct {
		Email json.RawMessage `json:"email"`
	}
	if json.Unmarshal(claims.Profile, &profile) != nil {
		return ""
	}
	return emailClaim(profile.Email)
}

func emailClaim(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var email string
	if json.Unmarshal(raw, &email) != nil {
		return ""
	}
	return strings.TrimSpace(email)
}
