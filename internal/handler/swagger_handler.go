package handler

import (
	"net/http"

	_ "spliteasy/docs" // Blank import is necessary for swagger registration
	httpSwagger "github.com/swaggo/http-swagger"
)

// SwaggerHandler returns an http.Handler that wraps the third-party Swagger UI provider.
func SwaggerHandler() http.Handler {
	return httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)
}
