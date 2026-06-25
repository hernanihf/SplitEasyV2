package handler

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger"
	_ "spliteasy/docs" // Blank import is necessary for swagger registration
)

// SwaggerHandler returns an http.Handler that wraps the third-party Swagger UI provider.
func SwaggerHandler() http.Handler {
	return httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)
}
