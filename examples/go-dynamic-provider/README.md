# Example Go Dynamic Governance Provider

This is a thin Go example provider for the dynamic governance HTTP-over-UDS contract.

It uses the reusable SDK runtime in `go/dynamicprovider`, so the example only contains provider-specific tool, grant, and resource logic.

## Run

```bash
cd cortex-governance-provider-sdk
go run ./examples/go-dynamic-provider -socket /var/run/cortex-governance/providers/example/provider.sock
```
