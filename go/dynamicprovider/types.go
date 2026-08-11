package dynamicprovider

import (
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	ProtocolVersion                = "v1"
	DynamicProviderSignatureHeader = "X-Cortex-Dynamic-Provider-Signature"
	DynamicProviderTimestampHeader = "X-Cortex-Dynamic-Provider-Timestamp"
	DynamicProviderKeyHeader       = "X-Cortex-Dynamic-Provider-Key"
)

type Meta struct {
	Name            string         `json:"name"`
	ProtocolVersion string         `json:"protocol_version"`
	DisplayName     string         `json:"display_name"`
	Description     string         `json:"description"`
	BindingSchema   *BindingSchema `json:"binding_schema,omitempty"`
}

// BindingType is a closed set of shapes a BindingInputSchema value can take.
// Being a named type (rather than plain string) gives provider authors
// compile-time autocompletion/type-checking against the BindingType*
// constants instead of hand-typing string literals.
type BindingType string

// Accepted values for BindingInputSchema.Type. An empty/unset Type is
// equivalent to BindingTypeString: free text, no validation. These mirror
// cortex-governance/go/governance's BindingType* constants.
const (
	BindingTypeString BindingType = ""
	BindingTypeBool   BindingType = "bool"
	BindingTypeNumber BindingType = "number"
	BindingTypeSelect BindingType = "select"
	// Deprecated: use BindingTypePicker with PickerConfig{Multiple: true} for
	// provider-backed choices. Multiselect remains supported for static legacy
	// schemas and persists a delimiter-separated string.
	BindingTypeMultiSelect BindingType = "multiselect"
	BindingTypePicker      BindingType = "picker"
)

type PickerConfig struct {
	Multiple  bool     `json:"multiple,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

type BindingInputSchema struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	Source       string        `json:"source"`
	Claim        string        `json:"claim,omitempty"`
	SettingKey   string        `json:"setting_key,omitempty"`
	Required     bool          `json:"required,omitempty"`
	Sensitive    bool          `json:"sensitive,omitempty"`
	Hash         bool          `json:"hash,omitempty"`
	Secret       bool          `json:"secret,omitempty"`
	EnvKey       string        `json:"env_key,omitempty"`
	SignatureKey string        `json:"signature_key,omitempty"`
	Type         BindingType   `json:"type,omitempty"`
	Options      []string      `json:"options,omitempty"`
	Picker       *PickerConfig `json:"picker,omitempty"`
}

type ConfirmationMode string

const (
	ConfirmationModeAlways ConfirmationMode = "always"
)

type ApprovalLifecycle string

const (
	ApprovalLifecycleEveryTime ApprovalLifecycle = "every_time"
	ApprovalLifecycleTask      ApprovalLifecycle = "task"
	ApprovalLifecycleSession   ApprovalLifecycle = "session"
)

type ProviderActionConfirmation struct {
	Mode ConfirmationMode `json:"mode"`
}

type ApprovalPreviewField struct {
	Pointer  string `json:"pointer"`
	Label    string `json:"label"`
	Format   string `json:"format,omitempty"`
	MaxChars int    `json:"max_chars,omitempty"`
}

type ProviderActionPolicy struct {
	Action               string                     `json:"action"`
	PermissionInput      string                     `json:"permission_input"`
	Confirmation         ProviderActionConfirmation `json:"confirmation"`
	Title                string                     `json:"title"`
	PreviewFields        []ApprovalPreviewField     `json:"preview_fields,omitempty"`
	MaxApprovalLifecycle ApprovalLifecycle          `json:"max_approval_lifecycle,omitempty"`
	ApprovalScopeFields  []string                   `json:"approval_scope_fields,omitempty"`
}

type BindingSchema struct {
	Inputs  []BindingInputSchema   `json:"inputs,omitempty"`
	Actions []ProviderActionPolicy `json:"actions,omitempty"`
}

type TenantPolicy struct {
	TenantID string `json:"tenant_id"`
	Subject  string `json:"subject"`
}

type Context struct {
	Authorization string        `json:"authorization,omitempty"`
	Policy        *TenantPolicy `json:"policy,omitempty"`
}
type PickerOperation string

const (
	PickerOperationList     PickerOperation = "list"
	PickerOperationValidate PickerOperation = "validate"
)

type PickerItem struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Data        any    `json:"data,omitempty"`
}
type PickerValidation struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}
type PickerInput struct {
	Context        Context           `json:"context"`
	Operation      PickerOperation   `json:"operation"`
	InputName      string            `json:"input_name"`
	TargetTenantID string            `json:"target_tenant_id,omitempty"`
	Dependencies   map[string]string `json:"dependencies,omitempty"`
	Query          string            `json:"query,omitempty"`
	Cursor         string            `json:"cursor,omitempty"`
	Limit          int               `json:"limit,omitempty"`
	SelectedValues []string          `json:"selected_values,omitempty"`
}
type PickerOutput struct {
	Items      []PickerItem      `json:"items,omitempty"`
	NextCursor string            `json:"next_cursor,omitempty"`
	Validation *PickerValidation `json:"validation,omitempty"`
}

type Request struct {
	Provider   string                 `json:"provider"`
	Action     string                 `json:"action"`
	ResourceID string                 `json:"resource_id"`
	Params     map[string]interface{} `json:"params,omitempty"`
}

type ProtectedResource struct {
	Provider     string `json:"provider"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

type CapabilityGrant struct {
	TenantID     string   `json:"tenant_id"`
	Provider     string   `json:"provider"`
	ResourceType string   `json:"resource_type"`
	ResourceID   string   `json:"resource_id"`
	Scopes       []string `json:"scopes"`
	ExpiresAt    int64    `json:"expires_at"`
	Signature    string   `json:"signature,omitempty"`
}

type ToolsResponse struct {
	Tools []mcp.Tool `json:"tools"`
}

type MapRequestInput struct {
	ToolName  string                 `json:"tool_name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type MapRequestOutput struct {
	Request *Request `json:"request"`
}

type ResolveResourceInput struct {
	Context Context  `json:"context"`
	Request *Request `json:"request"`
}

type ResolveResourceOutput struct {
	Resource *ProtectedResource `json:"resource"`
}

type MapScopeInput struct {
	Action string `json:"action"`
}

type MapScopeOutput struct {
	Scope string `json:"scope"`
}

type ExecuteInput struct {
	Context Context  `json:"context"`
	Request *Request `json:"request"`
}

type DelegatedDownload struct {
	ResourceID       string                 `json:"resource_id"`
	Filename         string                 `json:"filename,omitempty"`
	ContentType      string                 `json:"content_type,omitempty"`
	ExpiresInSeconds int                    `json:"expires_in_seconds,omitempty"`
	Params           map[string]interface{} `json:"params,omitempty"`
	NextStep         string                 `json:"next_step,omitempty"`
	// StageToFileStore, when true, asks the host to materialize this download and
	// stage it into the shared tenant file store, returning the file provider's
	// governed URLs (system_internal_provider_url/user_external_download_url) instead of a
	// per-provider delegated-download ticket. Prefer this for generated content so
	// it lands in the same store as every other provider's output.
	StageToFileStore bool `json:"stage_to_file_store,omitempty"`
	// Path is the optional relative destination within the tenant file store when
	// StageToFileStore is set. Leave empty to use a conventional .staging path.
	Path string `json:"path,omitempty"`
}

type ParseSettingsInput struct {
	TenantID string                 `json:"tenant_id"`
	Token    string                 `json:"token,omitempty"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

type ParseSettingsOutput struct {
	Grants []CapabilityGrant `json:"grants"`
}

type ComputeBindingInput struct {
	// Secret is deprecated and is now passed as an empty string. Binding signatures
	// are computed automatically on the host, so providers do not need the secret.
	Secret string                 `json:"secret"`
	Params map[string]interface{} `json:"params,omitempty"`
}

type BindingAttribute struct {
	Value     string `json:"value"`
	Signature string `json:"signature,omitempty"`
}

type BindingEnvelope struct {
	Bindings map[string]BindingAttribute `json:"bindings,omitempty"`
}

type ComputeBindingOutput struct {
	Binding  map[string]string `json:"binding,omitempty"`
	Envelope *BindingEnvelope  `json:"envelope,omitempty"`
}

type IsAvailableInput struct {
	Context  Context `json:"context"`
	TenantID string  `json:"tenant_id,omitempty"`
}

type FetchDownloadInput struct {
	Context  Context           `json:"context"`
	Download DelegatedDownload `json:"download"`
}

type FetchDownloadOutput struct {
	ContentBase64 string `json:"content_base64"`
	ContentType   string `json:"content_type,omitempty"`
	Filename      string `json:"filename,omitempty"`
}
