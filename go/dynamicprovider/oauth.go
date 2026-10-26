package dynamicprovider

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// InputSourceOAuth is the Source of a secret whose value is the tenant's OAuth
// access token. It references its OAuth client in BindingInputSchema.OAuth.
const InputSourceOAuth = "oauth"

// ProviderOAuthConfig is the OAuth 2.0 client a provider declares for an
// OAuth-backed secret (authorization code + PKCE). The endpoints and scopes
// ship with the provider; the tenant's own client credentials are ordinary
// inputs of the same provider that this config points at by name, so each
// tenant uses its own OAuth app. It mirrors cortex-governance's
// ProviderOAuthConfig.
type ProviderOAuthConfig struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	Scopes                string `json:"scopes,omitempty"`
	// TokenAuthMethod is how the client secret reaches the token endpoint:
	// "basic" (default) or "post". Ignored for a public client.
	TokenAuthMethod string `json:"token_auth_method,omitempty"`
	// AuthorizationParams are extra consent URL parameters some providers need
	// before they issue a refresh token (for example access_type=offline).
	AuthorizationParams map[string]string `json:"authorization_params,omitempty"`
	// ClientIDInput names the input holding the tenant's OAuth client id.
	ClientIDInput string `json:"client_id_input"`
	// ClientSecretInput optionally names the (sensitive) input holding the
	// client secret; leave empty for a public client.
	ClientSecretInput string `json:"client_secret_input,omitempty"`
}

// ErrOAuthNotConnected means the tenant has not connected the OAuth-backed
// secret, or its connection could not supply a token. The host reports an
// unusable connection as an error before the call, so in practice this is the
// "never connected" case; surface it so the admin is told to connect.
var ErrOAuthNotConnected = errors.New("oauth is not connected for this tenant")

// OAuthSecretInput declares a secret whose value is the tenant's OAuth access
// token. The returned input is sensitive and never a pasted setting.
func OAuthSecretInput(name, label, description string, cfg ProviderOAuthConfig) BindingInputSchema {
	c := cfg
	return BindingInputSchema{
		Name: name, Label: label, Description: description,
		Source: InputSourceOAuth, Type: BindingTypeSecret, Sensitive: true, OAuth: &c,
	}
}

// AccessToken returns the OAuth access token the host delivered for the named
// OAuth-backed secret, or ErrOAuthNotConnected when there is none.
func (c Context) AccessToken(inputName string) (string, error) {
	if token := strings.TrimSpace(c.Secrets[inputName]); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("%w: %s", ErrOAuthNotConnected, inputName)
}

func (i BindingInputSchema) isSensitive() bool {
	return i.Sensitive || i.Secret || i.Hash || i.Type == BindingTypeSecret
}

// reservedAuthorizationParams are consent URL parameters the host sets itself.
var reservedAuthorizationParams = map[string]bool{
	"response_type": true, "client_id": true, "redirect_uri": true, "state": true,
	"code_challenge": true, "code_challenge_method": true, "scope": true,
}

func oauthEndpointOK(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	return false
}

// ValidateOAuth checks every OAuth-backed secret of the schema with the same
// rules the host applies when it loads the provider, so a malformed declaration
// is caught when the provider starts or its tests run instead of when the host
// refuses to load it.
func (s *BindingSchema) ValidateOAuth() error {
	if s == nil {
		return nil
	}
	byName := make(map[string]BindingInputSchema, len(s.Inputs))
	for _, input := range s.Inputs {
		byName[input.Name] = input
	}
	for _, input := range s.Inputs {
		cfg := input.OAuth
		if cfg == nil {
			if strings.TrimSpace(input.Source) == InputSourceOAuth {
				return fmt.Errorf("input %q: source %q needs an oauth block", input.Name, InputSourceOAuth)
			}
			continue
		}
		if strings.TrimSpace(input.Source) != InputSourceOAuth || input.Type != BindingTypeSecret {
			return fmt.Errorf("input %q: an oauth block needs source %q and type %q", input.Name, InputSourceOAuth, BindingTypeSecret)
		}
		if !oauthEndpointOK(cfg.AuthorizationEndpoint) || !oauthEndpointOK(cfg.TokenEndpoint) {
			return fmt.Errorf("input %q: oauth authorization_endpoint and token_endpoint must be https URLs", input.Name)
		}
		switch strings.ToLower(strings.TrimSpace(cfg.TokenAuthMethod)) {
		case "", "basic", "post":
		default:
			return fmt.Errorf("input %q: oauth token_auth_method must be basic or post", input.Name)
		}
		for k := range cfg.AuthorizationParams {
			if k == "" || reservedAuthorizationParams[k] {
				return fmt.Errorf("input %q: authorization_params may not set %q", input.Name, k)
			}
		}
		clientID, ok := byName[cfg.ClientIDInput]
		if !ok || clientID.OAuth != nil {
			return fmt.Errorf("input %q: oauth client_id_input %q must name another input", input.Name, cfg.ClientIDInput)
		}
		if cfg.ClientSecretInput != "" {
			secret, ok := byName[cfg.ClientSecretInput]
			if !ok || secret.OAuth != nil || !secret.isSensitive() {
				return fmt.Errorf("input %q: oauth client_secret_input %q must name a sensitive input", input.Name, cfg.ClientSecretInput)
			}
		}
	}
	return nil
}
