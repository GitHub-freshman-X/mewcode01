package skills

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Manager struct {
	mu          sync.RWMutex
	options     DiscoverOptions
	catalog     Catalog
	activations []Activation
}

func NewManager(options DiscoverOptions) (*Manager, error) {
	catalog, err := Discover(options)
	if err != nil {
		return nil, err
	}
	return &Manager{options: options, catalog: catalog}, nil
}

func (m *Manager) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{Catalog: Catalog{Skills: map[string]Skill{}}}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Snapshot{Catalog: cloneCatalog(m.catalog), Activations: append([]Activation(nil), m.activations...)}
}

func (m *Manager) Activate(name, args string) (Skill, error) {
	if m == nil {
		return Skill{}, fmt.Errorf("skill manager is not configured")
	}
	name = strings.TrimSpace(name)
	m.mu.Lock()
	defer m.mu.Unlock()
	skill, ok := m.catalog.Skills[name]
	if !ok {
		return Skill{}, fmt.Errorf("unknown skill %q", name)
	}
	m.activations = append(m.activations, Activation{Name: name, Args: args})
	return skill, nil
}

func (m *Manager) ClearActivations() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.activations = nil
	m.mu.Unlock()
}

func (m *Manager) Reload() error {
	if m == nil {
		return fmt.Errorf("skill manager is not configured")
	}
	catalog, err := Discover(m.options)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.catalog = catalog
	m.mu.Unlock()
	return nil
}

func (m *Manager) Skill(name string) (Skill, bool) {
	if m == nil {
		return Skill{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	skill, ok := m.catalog.Skills[strings.TrimSpace(name)]
	if !ok {
		return Skill{}, false
	}
	skill.Tools = append([]string(nil), skill.Tools...)
	return skill, true
}

func (m *Manager) Directory() []Metadata {
	snapshot := m.Snapshot()
	names := make([]string, 0, len(snapshot.Catalog.Skills))
	for name := range snapshot.Catalog.Skills {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Metadata, 0, len(names))
	for _, name := range names {
		metadata := snapshot.Catalog.Skills[name].Metadata
		metadata.Tools = append([]string(nil), metadata.Tools...)
		result = append(result, metadata)
	}
	return result
}
