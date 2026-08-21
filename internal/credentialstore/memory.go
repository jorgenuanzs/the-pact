package credentialstore

import "sync"

// Memory is intended for deterministic tests. It never persists secrets.
type Memory struct {
	mu      sync.RWMutex
	secrets map[string]string
}

func NewMemory() *Memory {
	return &Memory{secrets: make(map[string]string)}
}

func (m *Memory) Put(reference, secret string) error {
	reference, err := validateReference(reference)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets[reference] = secret
	return nil
}

func (m *Memory) Get(reference string) (string, error) {
	reference, err := validateReference(reference)
	if err != nil {
		return "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	secret, ok := m.secrets[reference]
	if !ok {
		return "", ErrNotFound
	}
	return secret, nil
}

func (m *Memory) Delete(reference string) error {
	reference, err := validateReference(reference)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.secrets, reference)
	return nil
}

func (m *Memory) Exists(reference string) (bool, error) {
	_, err := m.Get(reference)
	if err == nil {
		return true, nil
	}
	if err == ErrNotFound {
		return false, nil
	}
	return false, err
}
