package apidoc_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/MErenTalan/ekokod-rewrite/internal/apidoc"
)

const tiny = `{
 "openapi": "3.1.0",
 "info": {"title": "ekokod API", "version": "1"},
 "paths": {
  "/api/v1/things/{id}": {
   "get": {"operationId": "things.get", "summary": "One thing.", "tags": ["things"],
    "security": [{"cookieAuth": []}],
    "parameters": [{"in": "path", "name": "id", "required": true, "schema": {"$ref": "#/components/schemas/UuidUUID"}},
                   {"in": "query", "name": "limit", "schema": {"type": ["null", "integer"]}}],
    "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/GithubComMErenTalanEkokodRewriteInternalApiV1DtoThing"}}}},
                  "404": {"description": "Not Found"}}}
  },
  "/api/v1/auth/login": {
   "post": {"operationId": "auth.login", "summary": "Log in.",
    "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Login"}}}},
    "responses": {"204": {"description": "No Content"}}}
  }
 },
 "components": {"schemas": {
  "GithubComMErenTalanEkokodRewriteInternalApiV1DtoThing": {"type": "object", "required": ["id"],
   "properties": {"id": {"type": "string", "format": "uuid"}, "tags": {"type": "array", "items": {"type": "string"}}, "next": {"$ref": "#/components/schemas/Login"}}},
  "Login": {"type": "object", "properties": {"email": {"type": "string"}}}
 }}
}`

const want = "# ekokod API reference\n\n" +
	"Generated from the OpenAPI document (`ekokod tool openapi`) by `make api-docs`; do not edit.\n" +
	"Version 1 · 2 operations · 2 schemas. Authentication: a session cookie (`ekokod_at`) or a mobile bearer token; operations marked *public* need neither.\n" +
	"Errors 400, 401, 403, 429 and 500 apply to every operation and use the `Error` schema.\n\n" +
	"## auth\n\n" +
	"### `POST /api/v1/auth/login`\n\nLog in. *public* · `auth.login`\n\n" +
	"Body: [`Login`](#schema-login)\n\nResponses: 204\n\n" +
	"## things\n\n" +
	"### `GET /api/v1/things/{id}`\n\nOne thing. `things.get`\n\n" +
	"| parameter | in | required | type |\n|---|---|---|---|\n" +
	"| `id` | path | yes | `UuidUUID` |\n| `limit` | query |  | integer? |\n\n" +
	"Responses: 200 [`Thing`](#schema-thing), 404\n\n" +
	"## Schemas\n\n" +
	"### <a id=\"schema-login\"></a>`Login`\n\n| field | type | required |\n|---|---|---|\n| `email` | string |  |\n\n" +
	"### <a id=\"schema-thing\"></a>`Thing`\n\n| field | type | required |\n|---|---|---|\n" +
	"| `id` | string (uuid) | yes |\n| `next` | [`Login`](#schema-login) |  |\n| `tags` | string[] |  |\n\n"

func TestMarkdownRendersOperationsAndSchemas(t *testing.T) {
	got, err := apidoc.Markdown([]byte(tiny))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestMarkdownCoversTheRealDocument(t *testing.T) {
	doc, err := v1.BuildOpenAPI(v1.Table())
	require.NoError(t, err)
	md, err := apidoc.Markdown(doc)
	require.NoError(t, err)
	for _, path := range []string{"/api/v1/consumption", "/api/v1/auth/login", "/api/v1/alarms/{id}/events"} {
		require.Contains(t, string(md), " "+path+"`\n")
	}
	require.False(t, strings.Contains(string(md), "GithubCom"), "schema names are shortened")
}
