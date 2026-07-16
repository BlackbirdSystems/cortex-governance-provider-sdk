package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/blackbirdsystems/cortex-governance-provider-sdk/go/dynamicprovider"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	providerName        = "example"
	providerDisplayName = "Example Dynamic Provider"
	providerDescription = "Reference HTTP-over-UDS governance provider"
)

func main() {
	socketPath := flag.String("socket", "/var/run/cortex-governance/providers/example/provider.sock", "Unix socket path")
	flag.Parse()

	secret, err := dynamicprovider.SecretFromEnv()
	if err != nil {
		log.Fatalf("dynamic provider secret: %v", err)
	}

	server := dynamicprovider.NewServer(
		dynamicprovider.Meta{
			Name:            providerName,
			ProtocolVersion: dynamicprovider.ProtocolVersion,
			DisplayName:     providerDisplayName,
			Description:     providerDescription,
			BindingSchema: &dynamicprovider.BindingSchema{
				Inputs: []dynamicprovider.BindingInputSchema{
					{
						Name:         "example_api_enabled",
						Source:       "settings",
						SettingKey:   "example_api_enabled",
						Required:     true,
						SignatureKey: "example_api_sig",
						Description:  "Set to \"true\" to enable the example provider and generate a binding signature.",
					},
					{
						Name:        "example_api_sig",
						Source:      "computed",
						Sensitive:   true,
						Description: "Binding signature for example_api_enabled. Computed automatically on generate.",
					},
					{
						Name:         "example_teams",
						Source:       "settings",
						SettingKey:   "example_teams",
						Required:     false,
						SignatureKey: "example_team_sig",
						Description:  "Optional comma-separated team names to scope access.",
					},
					{
						Name:        "example_team_sig",
						Source:      "computed",
						Sensitive:   true,
						Description: "Team-scoped binding signature. Computed automatically from example_teams.",
					},
				},
			},
		},
		secret,
		dynamicprovider.Callbacks{
			Tools:           tools,
			MapRequest:      mapRequest,
			ResolveResource: resolveResource,
			MapScope:        mapScope,
			Execute:         execute,
			ParseSettings:   parseSettings,
			ComputeBinding:  computeBinding,
			IsAvailable:     isAvailable,
			FetchDownload:   fetchDownload,
		},
	)

	log.Printf("example dynamic governance provider listening on %s", *socketPath)
	if err := server.ServeUDS(*socketPath); err != nil {
		log.Fatalf("serve provider: %v", err)
	}
}

func tools() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("example_echo",
			mcp.WithDescription("Echo a message through the dynamic governance provider"),
			mcp.WithString("message", mcp.Description("Message to echo back"), mcp.Required()),
		),
		mcp.NewTool("example_get_echo_download_url",
			mcp.WithDescription("Return an agent-owned governed download URL delegated back to this provider"),
			mcp.WithString("message", mcp.Description("Message to download as a text file"), mcp.Required()),
		),
	}
}

func mapRequest(input dynamicprovider.MapRequestInput) (dynamicprovider.MapRequestOutput, error) {
	return dynamicprovider.MapRequestOutput{
		Request: &dynamicprovider.Request{
			Provider:   providerName,
			Action:     actionForTool(input.ToolName),
			ResourceID: "example-resource",
			Params:     input.Arguments,
		},
	}, nil
}

func resolveResource(input dynamicprovider.ResolveResourceInput) (dynamicprovider.ResolveResourceOutput, error) {
	return dynamicprovider.ResolveResourceOutput{
		Resource: &dynamicprovider.ProtectedResource{
			Provider:     providerName,
			ResourceType: "echo_resource",
			ResourceID:   input.Request.ResourceID,
		},
	}, nil
}

func mapScope(input dynamicprovider.MapScopeInput) (dynamicprovider.MapScopeOutput, error) {
	return dynamicprovider.MapScopeOutput{Scope: "invoke_" + input.Action}, nil
}

func execute(input dynamicprovider.ExecuteInput) (map[string]any, error) {
	if input.Request.Action == "get_echo_download" {
		return map[string]any{
			"delegated_download": dynamicprovider.DelegatedDownload{
				ResourceID:       "example-echo-download",
				Filename:         "example-echo.txt",
				ContentType:      "text/plain; charset=utf-8",
				ExpiresInSeconds: 300,
				Params: map[string]any{
					"message": input.Request.Params["message"],
				},
				NextStep: "Use downloadUrl to fetch the provider-generated text file.",
			},
		}, nil
	}

	response := map[string]any{
		"provider":       providerName,
		"message":        input.Request.Params["message"],
		"tenant_id":      "",
		"policy_subject": "",
	}
	if input.Context.Policy != nil {
		response["tenant_id"] = input.Context.Policy.TenantID
		response["policy_subject"] = input.Context.Policy.Subject
	}
	return response, nil
}

func parseSettings(input dynamicprovider.ParseSettingsInput) (dynamicprovider.ParseSettingsOutput, error) {
	sig, ok := input.Settings["example_api_sig"].(string)
	if !ok || strings.TrimSpace(sig) == "" {
		return dynamicprovider.ParseSettingsOutput{}, fmt.Errorf("unauthorized settings: example_api_sig is required")
	}

	return dynamicprovider.ParseSettingsOutput{
		Grants: []dynamicprovider.CapabilityGrant{
			{
				TenantID:     input.TenantID,
				Provider:     providerName,
				ResourceType: "echo_resource",
				ResourceID:   "*",
				Scopes:       []string{"invoke_echo"},
				ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
			},
		},
	}, nil
}

func computeBinding(input dynamicprovider.ComputeBindingInput) (dynamicprovider.ComputeBindingOutput, error) {
	bindingAttributes := map[string]string{}
	for key, value := range input.Params {
		if key == "tenant_id" || strings.HasSuffix(key, "_sig") {
			continue
		}
		bindingAttributes[key] = fmt.Sprint(value)
	}

	if len(bindingAttributes) > 1 {
		envelope := &dynamicprovider.BindingEnvelope{Bindings: map[string]dynamicprovider.BindingAttribute{}}
		for key, value := range bindingAttributes {
			// No HMAC signature is generated by the provider anymore; signatures are managed by the host.
			envelope.Bindings[key] = dynamicprovider.BindingAttribute{
				Value: value,
			}
		}
		return dynamicprovider.ComputeBindingOutput{Envelope: envelope}, nil
	}

	binding := map[string]string{}
	for k, v := range bindingAttributes {
		binding[k] = v
	}
	return dynamicprovider.ComputeBindingOutput{Binding: binding}, nil
}

func isAvailable(input dynamicprovider.IsAvailableInput) (bool, error) {
	return true, nil
}

func fetchDownload(input dynamicprovider.FetchDownloadInput) (dynamicprovider.FetchDownloadOutput, error) {
	message, _ := input.Download.Params["message"].(string)
	return dynamicprovider.FetchDownloadOutput{
		ContentBase64: base64.StdEncoding.EncodeToString([]byte(message)),
		ContentType:   "text/plain; charset=utf-8",
		Filename:      "example-echo.txt",
	}, nil
}

func actionForTool(toolName string) string {
	switch toolName {
	case "example_get_echo_download_url":
		return "get_echo_download"
	default:
		return "echo"
	}
}
