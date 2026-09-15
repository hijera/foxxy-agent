package webauth

// The environment variables that carry the HTTP surface's credentials. They
// live here rather than in external/httpserver, so every reader uses the same
// names without importing the server: `foxxycode serve` and `foxxycode http`,
// a binary built without the http tag, and the dry run, which the server
// itself imports.
const (
	// TokenEnvVar is the bearer token for API clients.
	TokenEnvVar = "FOXXYCODE_HTTP_TOKEN"
	// LoginUserEnvVar and LoginPasswordEnvVar are the web sign-in account. The
	// password is plaintext, hashed as the server starts and never written to
	// config.yaml, which is why $FOXXYCODE_HOME/.env is its natural home.
	LoginUserEnvVar     = "FOXXYCODE_HTTP_USER"
	LoginPasswordEnvVar = "FOXXYCODE_HTTP_PASSWORD"
)
