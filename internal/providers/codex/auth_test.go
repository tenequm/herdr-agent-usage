/**
 * Tests for reading the Codex account identity out of auth.json.
 */
package codex

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeCodexAuth(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestAccountID(t *testing.T) {
	// The real shape: the id sits under tokens, beside secrets this must not
	// touch.
	const real = `{
		"last_refresh": "2026-07-25T12:30:49Z",
		"OPENAI_API_KEY": null,
		"tokens": {
			"access_token": "secret-access",
			"account_id": "a1afefbc-024d-46be-beb3-add8311de670",
			"id_token": "secret-id",
			"refresh_token": "secret-refresh"
		}
	}`

	cases := []struct {
		name string
		body string
		want string
	}{
		{"real auth.json", real, "a1afefbc-024d-46be-beb3-add8311de670"},
		{"surrounding whitespace trimmed", `{"tokens":{"account_id":"  abc-123  "}}`, "abc-123"},
		{"api-key login has no account id", `{"OPENAI_API_KEY":"sk-x","tokens":null}`, ""},
		{"tokens without account_id", `{"tokens":{"access_token":"x"}}`, ""},
		{"account_id is null", `{"tokens":{"account_id":null}}`, ""},
		{"empty object", `{}`, ""},
		{"unparseable json", `{"tokens":`, ""},
		{"empty file", ``, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CODEX_HOME", writeCodexAuth(t, tc.body))
			if got := AccountID(); got != tc.want {
				t.Fatalf("AccountID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAccountIDWithoutAuthFile(t *testing.T) {
	// Never signed in here: the caller must get "" so a borrowed rate-limit
	// window falls back to the pool's own single-account rule instead of
	// being matched against a fabricated identity.
	t.Setenv("CODEX_HOME", t.TempDir())
	if got := AccountID(); got != "" {
		t.Fatalf("AccountID() = %q, want empty", got)
	}
}

func TestAccountIDNeverReturnsSecrets(t *testing.T) {
	t.Setenv("CODEX_HOME", writeCodexAuth(t, `{
		"tokens": {
			"access_token": "sk-super-secret",
			"id_token": "jwt-secret",
			"refresh_token": "rt-secret",
			"account_id": "acct-1"
		}
	}`))
	got := AccountID()
	if got != "acct-1" {
		t.Fatalf("AccountID() = %q, want %q", got, "acct-1")
	}
	for _, secret := range []string{"sk-super-secret", "jwt-secret", "rt-secret"} {
		if got == secret {
			t.Fatalf("AccountID() leaked %q", secret)
		}
	}
}

func testIDToken(t *testing.T, claims any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + ".test-signature"
}

func authWithIDToken(t *testing.T, token string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"tokens": map[string]any{"id_token": token},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAccountEmailIn(t *testing.T) {
	tests := []struct {
		name   string
		claims any
		want   string
	}{
		{"standard claim", map[string]any{"email": "person@example.com"}, "person@example.com"},
		{"profile claim", map[string]any{
			"https://api.openai.com/profile": map[string]any{"email": "nested@example.com"},
		}, "nested@example.com"},
		{"standard claim wins", map[string]any{
			"email":                          "standard@example.com",
			"https://api.openai.com/profile": map[string]any{"email": "nested@example.com"},
		}, "standard@example.com"},
		{"whitespace trimmed", map[string]any{"email": "  person@example.com  "}, "person@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := writeCodexAuth(t, authWithIDToken(t, testIDToken(t, tt.claims)))
			if got := AccountEmailIn(home); got != tt.want {
				t.Fatalf("AccountEmailIn() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAccountEmailInMalformedOrMissingToken(t *testing.T) {
	tests := []string{
		`{}`,
		`{"tokens":null}`,
		`{"tokens":{}}`,
		`{"tokens":{"id_token":null}}`,
		`{"tokens":{"id_token":"not-a-jwt"}}`,
		`{"tokens":{"id_token":"e30.invalid!.signature"}}`,
		`{"tokens":{"id_token":"e30.bm90LWpzb24.signature"}}`,
		authWithIDToken(t, testIDToken(t, map[string]any{"sub": "account"})),
		authWithIDToken(t, testIDToken(t, map[string]any{"email": ""})),
	}
	for _, body := range tests {
		if got := AccountEmailIn(writeCodexAuth(t, body)); got != "" {
			t.Fatalf("AccountEmailIn() = %q for %s, want empty", got, body)
		}
	}
	if got := AccountEmailIn(t.TempDir()); got != "" {
		t.Fatalf("missing auth.json returned %q", got)
	}
}
