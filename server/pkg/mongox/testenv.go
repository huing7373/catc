package mongox

import "os"

// IntegrationURI returns the CAT_TEST_MONGO_URI environment value, or
// an empty string if unset. Concentrating this os.Getenv in a single
// place keeps the architecture rule (no env reads outside config) easy
// to enforce by grep — both production code and integration tests reach
// the env via this helper instead of scattering os.Getenv across the
// repository.
func IntegrationURI() string { return os.Getenv("CAT_TEST_MONGO_URI") }
