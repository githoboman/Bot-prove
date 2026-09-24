package keystore

import (
	"fmt"
	"os"
	"strings"

	pqcrypto "github.com/anna-stolbovskaja/CasperProver/engine/internal/crypto"
)

// FromEnv constructs the Keystore backend selected by environment vars.
//
// Selection:
//
//   CP_KEYSTORE_KIND=memory        (default)             -> MemoryKeystore
//   CP_KEYSTORE_KIND=file          + CP_KEYSTORE_PATH    -> FileKeystore
//                                  + CP_KEYSTORE_PASSPHRASE
//   CP_KEYSTORE_KIND=remote        + CP_KEYSTORE_URL     -> RemoteKeystoreStub
//                                  + CP_KEYSTORE_TOKEN
//   CP_KEYSTORE_KIND=vault-transit + CP_VAULT_ADDR       -> VaultTransitKeystore
//                                  + CP_VAULT_TOKEN
//                                  + [CP_VAULT_TRANSIT_MOUNT=transit]
//
// Any parse/setup error is returned; the caller (server bootstrap) decides
// whether to fall back to MemoryKeystore or fail-fast.
func FromEnv() (Keystore, string, error) {
	kind := strings.ToLower(strings.TrimSpace(os.Getenv("CP_KEYSTORE_KIND")))
	if kind == "" {
		kind = string(KindMemory)
	}
	switch Kind(kind) {
	case KindMemory:
		return NewMemory(pqcrypto.NewKeyRing()), "memory (default)", nil
	case KindFile:
		path := os.Getenv("CP_KEYSTORE_PATH")
		pass := os.Getenv("CP_KEYSTORE_PASSPHRASE")
		if path == "" || pass == "" {
			return nil, "", fmt.Errorf("%w: file backend needs CP_KEYSTORE_PATH and CP_KEYSTORE_PASSPHRASE", ErrNotConfigured)
		}
		fk, err := NewFile(path, []byte(pass))
		if err != nil {
			return nil, "", err
		}
		return fk, "file at " + path, nil
	case KindRemote:
		url := os.Getenv("CP_KEYSTORE_URL")
		token := os.Getenv("CP_KEYSTORE_TOKEN")
		return NewRemote(url, token), "remote gateway " + url, nil
	case KindVaultTransit:
		addr := os.Getenv("CP_VAULT_ADDR")
		token := os.Getenv("CP_VAULT_TOKEN")
		mount := os.Getenv("CP_VAULT_TRANSIT_MOUNT")
		if addr == "" || token == "" {
			return nil, "", fmt.Errorf("%w: vault-transit backend needs CP_VAULT_ADDR and CP_VAULT_TOKEN", ErrNotConfigured)
		}
		return NewVaultTransit(addr, token, mount), "vault-transit at " + addr, nil
	default:
		return nil, "", fmt.Errorf("keystore: unknown CP_KEYSTORE_KIND=%q (memory|file|remote|vault-transit)", kind)
	}
}
