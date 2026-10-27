package dynamicprovider

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func qbSchema() *BindingSchema {
	return &BindingSchema{Inputs: []BindingInputSchema{
		{Name: "client_id", Source: "settings", Label: "Client ID"},
		{Name: "client_secret", Source: "settings", Type: BindingTypeSecret},
		OAuthSecretInput("access_token", "QuickBooks", "Connect your QuickBooks company", ProviderOAuthConfig{
			AuthorizationEndpoint: "https://appcenter.intuit.com/connect/oauth2",
			TokenEndpoint:         "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer",
			Scopes:                "com.intuit.quickbooks.accounting",
			ClientIDInput:         "client_id", ClientSecretInput: "client_secret",
		}),
	}}
}

func TestOAuthSecretInputIsASensitiveOAuthSourcedSecret(t *testing.T) {
	in := qbSchema().Inputs[2]
	if in.Source != InputSourceOAuth || in.Type != BindingTypeSecret || !in.Sensitive || in.OAuth == nil || in.Label != "QuickBooks" {
		t.Fatalf("input = %+v", in)
	}
}

func TestOAuthDeclarationMatchesTheHostWireFormat(t *testing.T) {
	raw, err := json.Marshal(qbSchema().Inputs[2])
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	oauth, _ := wire["oauth"].(map[string]any)
	if wire["source"] != "oauth" || wire["type"] != "secret" || oauth["client_id_input"] != "client_id" || oauth["client_secret_input"] != "client_secret" || oauth["token_endpoint"] == nil {
		t.Fatalf("wire = %s", raw)
	}
}

func TestContextAccessToken(t *testing.T) {
	c := Context{Secrets: map[string]string{"access_token": " tok "}}
	if got, err := c.AccessToken("access_token"); err != nil || got != "tok" {
		t.Fatalf("AccessToken = %q, %v", got, err)
	}
	for _, empty := range []Context{{}, {Secrets: map[string]string{"access_token": "  "}}} {
		if _, err := empty.AccessToken("access_token"); !errors.Is(err, ErrOAuthNotConnected) {
			t.Fatalf("err = %v, want ErrOAuthNotConnected", err)
		}
	}
}

func TestContextSecretsDecodeFromTheHostPayload(t *testing.T) {
	var in ExecuteInput
	if err := json.Unmarshal([]byte(`{"context":{"secrets":{"access_token":"at-1"}},"request":{}}`), &in); err != nil {
		t.Fatal(err)
	}
	if got, err := in.Context.AccessToken("access_token"); err != nil || got != "at-1" {
		t.Fatalf("AccessToken = %q, %v", got, err)
	}
}

func TestValidateOAuth(t *testing.T) {
	if err := qbSchema().ValidateOAuth(); err != nil {
		t.Fatalf("good schema rejected: %v", err)
	}
	if err := (*BindingSchema)(nil).ValidateOAuth(); err != nil {
		t.Fatalf("nil schema rejected: %v", err)
	}
	for name, mutate := range map[string]func(*BindingSchema){
		"wrong type":                 func(s *BindingSchema) { s.Inputs[2].Type = BindingTypeString },
		"wrong source":               func(s *BindingSchema) { s.Inputs[2].Source = "settings" },
		"oauth source without block": func(s *BindingSchema) { s.Inputs[2].OAuth = nil },
		"http token endpoint":        func(s *BindingSchema) { s.Inputs[2].OAuth.TokenEndpoint = "http://a.example/token" },
		"unknown client id input":    func(s *BindingSchema) { s.Inputs[2].OAuth.ClientIDInput = "nope" },
		"client id is oauth":         func(s *BindingSchema) { s.Inputs[0].OAuth = s.Inputs[2].OAuth },
		"non-sensitive secret input": func(s *BindingSchema) { s.Inputs[1].Type = BindingTypeString },
		"bad auth method":            func(s *BindingSchema) { s.Inputs[2].OAuth.TokenAuthMethod = "jwt" },
		"reserved param":             func(s *BindingSchema) { s.Inputs[2].OAuth.AuthorizationParams = map[string]string{"state": "x"} },
	} {
		s := qbSchema()
		mutate(s)
		if err := s.ValidateOAuth(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNewServerPanicsOnAMalformedOAuthDeclaration(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "client_id_input") {
			t.Fatalf("recover = %v, want a panic naming client_id_input", r)
		}
	}()
	s := qbSchema()
	s.Inputs[2].OAuth.ClientIDInput = "nope"
	NewServer(Meta{Name: "qb", BindingSchema: s}, []byte("secret"), Callbacks{})
}

func TestNewServerAcceptsAValidOAuthDeclaration(t *testing.T) {
	if NewServer(Meta{Name: "qb", BindingSchema: qbSchema()}, []byte("secret"), Callbacks{}) == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestContextOAuthParam(t *testing.T) {
	ctx := Context{OAuthParams: map[string]map[string]string{"access_token": {"realmId": " 9341 "}}}
	if got := ctx.OAuthParam("access_token", "realmId"); got != "9341" {
		t.Fatalf("OAuthParam = %q, want 9341", got)
	}
	if got := ctx.OAuthParam("access_token", "other"); got != "" {
		t.Fatalf("missing param = %q, want empty", got)
	}
	if got := (Context{}).OAuthParam("access_token", "realmId"); got != "" {
		t.Fatalf("empty context = %q, want empty", got)
	}
}
