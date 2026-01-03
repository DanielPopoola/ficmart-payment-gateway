package api

import (
	"encoding/json"
	"net/http"
)

// RegisterDocsRoutes registers documentation routes on the given mux.
//
// GET /            → Redirect to /docs
//
// GET /docs         → Swagger UI
//
// GET /docs/openapi → OpenAPI spec (JSON)
func RegisterDocsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", handleRootRedirect)
	mux.HandleFunc("/docs", handleSwaggerUI)
	mux.HandleFunc("/docs/openapi", handleOpenAPISpec)
}

func handleRootRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/docs", http.StatusMovedPermanently)
}

func handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	spec, err := GetSwagger()
	if err != nil {
		http.Error(w, "Failed to load OpenAPI spec", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(spec); err != nil {
		http.Error(w, "Failed to encode OpenAPI spec", http.StatusInternalServerError)
	}
}

func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIHTML)) //nolint:errcheck // Nothing useful to do if write fails
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Bank API - Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; padding: 0; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = () => {
      SwaggerUIBundle({
        url: '/docs/openapi',
        dom_id: '#swagger-ui',
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
        layout: 'StandaloneLayout'
      });
    };
  </script>
</body>
</html>`
