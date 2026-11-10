package account

import "runtime"

const maxVaultSize = 64 << 10

// CredentialsPersist reports whether the platform provides encrypted storage.
func CredentialsPersist() bool { return runtime.GOOS == "windows" }

// Vault persists provider credentials under an application data directory.
// Existing filenames, JSON payloads and Windows user encryption are preserved.
// Unsupported platforms retain credentials only in the caller's memory.
type Vault struct{ Root string }
