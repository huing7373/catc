package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestOpenAPISpec_StructurallyValid enforces the Story 0.14 AC9 CI gate:
// docs/api/openapi.yaml must parse as YAML and carry the OpenAPI 3.0.x
// markers + the /v1/platform/ws-registry endpoint with response schemas that
// match handler.WSRegistryResponse on the wire.
//
// This replaces the `swagger validate` CLI step envisioned in AC9: the
// go-swagger v0.31.0 CLI only validates Swagger 2.0, and the spec is
// OpenAPI 3.0.3 per AC8. Doing validation as a Go test keeps the same CI
// severity (test failure blocks the build) without a new external toolchain.
func TestOpenAPISpec_StructurallyValid(t *testing.T) {
	t.Parallel()

	specPath := filepath.Join("..", "..", "..", "docs", "api", "openapi.yaml")
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err, "open openapi.yaml: %s", specPath)

	var doc struct {
		OpenAPI string `yaml:"openapi"`
		Info    struct {
			Title   string `yaml:"title"`
			Version string `yaml:"version"`
		} `yaml:"info"`
		Paths      map[string]yaml.Node `yaml:"paths"`
		Components struct {
			Schemas map[string]yaml.Node `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc), "openapi.yaml must be valid YAML")

	assert.Regexp(t, `^3\.0\.\d+$`, doc.OpenAPI,
		"openapi field must declare OpenAPI 3.0.x (AC8)")
	assert.NotEmpty(t, doc.Info.Title, "info.title required")
	assert.NotEmpty(t, doc.Info.Version, "info.version required")

	require.Contains(t, doc.Paths, "/v1/platform/ws-registry",
		"spec must document the Story 0.14 AC5 endpoint")
	require.Contains(t, doc.Components.Schemas, "WSRegistryResponse")
	require.Contains(t, doc.Components.Schemas, "WSRegistryMessage")
}

// TestOpenAPISpec_SchemaFieldsMatchWireShape asserts the yaml schema names
// carry the exact camelCase JSON tags produced by handler.WSRegistryResponse
// / WSRegistryMessage. Drift here is the G2 failure mode AC8/AC9 are meant
// to catch (spec says `api_version`, code marshals `apiVersion`, clients
// break silently).
func TestOpenAPISpec_SchemaFieldsMatchWireShape(t *testing.T) {
	t.Parallel()

	specPath := filepath.Join("..", "..", "..", "docs", "api", "openapi.yaml")
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)

	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]yaml.Node `yaml:"properties"`
				Required   []string             `yaml:"required"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc))

	resp, ok := doc.Components.Schemas["WSRegistryResponse"]
	require.True(t, ok, "WSRegistryResponse schema must exist")
	for _, field := range []string{"apiVersion", "serverTime", "messages"} {
		assert.Contains(t, resp.Properties, field,
			"WSRegistryResponse.%s (camelCase) required — must match handler.WSRegistryResponse JSON tag", field)
		assert.Contains(t, resp.Required, field,
			"WSRegistryResponse must mark %q required", field)
	}

	msg, ok := doc.Components.Schemas["WSRegistryMessage"]
	require.True(t, ok)
	for _, field := range []string{"type", "version", "direction", "requiresAuth", "requiresDedup"} {
		assert.Contains(t, msg.Properties, field,
			"WSRegistryMessage.%s required — must match handler.WSRegistryMessage JSON tag", field)
		assert.Contains(t, msg.Required, field,
			"WSRegistryMessage must mark %q required", field)
	}
}
