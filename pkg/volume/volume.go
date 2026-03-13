package volume

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cryruss/cryruss/pkg/config"
	"github.com/cryruss/cryruss/pkg/store"
)

type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	CreatedAt  time.Time         `json:"CreatedAt"`
	Labels     map[string]string `json:"Labels"`
	Options    map[string]string `json:"Options"`
	Scope      string            `json:"Scope"`
}

type CreateRequest struct {
	Name    string            `json:"Name"`
	Driver  string            `json:"Driver"`
	Labels  map[string]string `json:"Labels"`
	Options map[string]string `json:"DriverOpts"`
}

type Manager struct {
	store *store.Store
}

func NewManager() *Manager {
	return &Manager{store: store.New(config.Global.VolumesDir)}
}

func (m *Manager) Create(req *CreateRequest) (*Volume, error) {
	name := req.Name
	if name == "" {
		name = store.NewID()[:16]
	}

	if m.store.Exists(name) {
		var v Volume
		m.store.Load(name, &v)
		return &v, nil
	}

	mountpoint := filepath.Join(config.Global.VolumesDir, name, "_data")
	if err := os.MkdirAll(mountpoint, 0755); err != nil {
		return nil, err
	}

	v := &Volume{
		Name:       name,
		Driver:     "local",
		Mountpoint: mountpoint,
		CreatedAt:  time.Now(),
		Labels:     req.Labels,
		Options:    req.Options,
		Scope:      "local",
	}
	if v.Labels == nil {
		v.Labels = map[string]string{}
	}
	if v.Options == nil {
		v.Options = map[string]string{}
	}

	if err := m.store.Save(name, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (m *Manager) Get(name string) (*Volume, error) {
	if !m.store.Exists(name) {
		return nil, fmt.Errorf("no such volume: %s", name)
	}
	var v Volume
	if err := m.store.Load(name, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (m *Manager) List() ([]*Volume, error) {
	ids, err := m.store.List()
	if err != nil {
		return nil, err
	}
	var result []*Volume
	for _, id := range ids {
		var v Volume
		if err := m.store.Load(id, &v); err != nil {
			continue
		}
		result = append(result, &v)
	}
	return result, nil
}

func (m *Manager) Delete(name string, force bool) error {
	v, err := m.Get(name)
	if err != nil {
		return err
	}
	os.RemoveAll(filepath.Dir(v.Mountpoint))
	return m.store.Delete(name)
}

func (m *Manager) Prune() ([]string, int64, error) {
	volumes, err := m.List()
	if err != nil {
		return nil, 0, err
	}
	var removed []string
	var freed int64
	for _, v := range volumes {
		size := dirSize(v.Mountpoint)
		if err := m.Delete(v.Name, false); err == nil {
			removed = append(removed, v.Name)
			freed += size
		}
	}
	return removed, freed, nil
}

func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
