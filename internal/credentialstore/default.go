package credentialstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const BackendEnvironment = "PACT_CREDENTIAL_STORE"

var testMemoryStores sync.Map

// Default selects the native system store. The file and memory backends are
// opt-in so PACT never silently downgrades credential protection.
func Default(configDirectory string) (Store, error) {
	switch backend := strings.ToLower(strings.TrimSpace(os.Getenv(BackendEnvironment))); backend {
	case "", "system":
		return NewSystem(), nil
	case "file":
		return NewFile(filepath.Join(configDirectory, "credentials"))
	case "memory":
		key, err := filepath.Abs(configDirectory)
		if err != nil {
			return nil, fmt.Errorf("resolve in-memory credential namespace: %w", err)
		}
		store, _ := testMemoryStores.LoadOrStore(key, NewMemory())
		return store.(*Memory), nil
	default:
		return nil, fmt.Errorf("unsupported %s value %q; use system, file, or memory", BackendEnvironment, backend)
	}
}
