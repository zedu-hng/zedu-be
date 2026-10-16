package config

type Firebase struct {
	ServiceFilePath string
	// CredentialsJSON is the service account JSON, raw or base64-encoded. Takes precedence over ServiceFilePath.
	CredentialsJSON string
}
