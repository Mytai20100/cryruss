package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cryruss/cryruss/pkg/config"
	"github.com/cryruss/cryruss/pkg/store"
)

type Manager struct {
	store *store.Store
}

func NewManager() *Manager {
	return &Manager{store: store.New(config.Global.ContainersDir)}
}

func (m *Manager) Create(req *CreateRequest, name string) (*Container, error) {
	id := store.NewID()

	if name == "" {
		name = generateName(id)
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	

	existing, _ := m.ListAll()
	for _, c := range existing {
		for _, n := range c.Names {
			if n == name {
				if c.State.Running {
					

					return nil, fmt.Errorf("name %q is already in use by container %s (running); stop it first or use a different name", name, c.ID[:12])
				}
				

				

				containerDir := filepath.Join(config.Global.ContainersDir, c.ID)
				os.RemoveAll(containerDir)
				os.Remove(c.LogPath)
				_ = m.store.Delete(c.ID)
			}
		}
	}

	rootfs := filepath.Join(config.Global.ContainersDir, id, "rootfs")
	logPath := filepath.Join(config.Global.LogsDir, id+".log")

	c := &Container{
		ID:         id,
		Names:      []string{name},
		Image:      req.Image,
		ImageID:    req.ImageID,
		Created:    time.Now().Unix(),
		RootfsPath: rootfs,
		LogPath:    logPath,
		Labels:     req.Labels,
		State: State{
			Status: StatusCreated,
		},
		Config: Config{
			Hostname:     req.Hostname,
			User:         req.User,
			Tty:          req.Tty,
			OpenStdin:    req.OpenStdin,
			AttachStdin:  req.AttachStdin,
			AttachStdout: req.AttachStdout,
			AttachStderr: req.AttachStderr,
			Env:          req.Env,
			Cmd:          req.Cmd,
			Image:        req.Image,
			WorkingDir:   req.WorkingDir,
			Entrypoint:   req.Entrypoint,
			Labels:       req.Labels,
			StopSignal:   req.StopSignal,
			StopTimeout:  req.StopTimeout,
			ExposedPorts: req.ExposedPorts,
			Volumes:      req.Volumes,
		},
		HostConfig:      req.HostConfig,
		NetworkSettings: NetworkSettings{Networks: map[string]*EndpointSettings{}},
		Mounts:          []Mount{},
		Ports:           []PortInfo{},
	}

	if c.Config.Hostname == "" {
		c.Config.Hostname = store.ShortID(id)
	}
	if c.Config.StopSignal == "" {
		c.Config.StopSignal = "SIGTERM"
	}
	if c.Config.StopTimeout == 0 {
		c.Config.StopTimeout = 10
	}
	if c.HostConfig.NetworkMode == "" {
		c.HostConfig.NetworkMode = "host"
	}

	os.MkdirAll(rootfs, 0755)

	if err := m.store.Save(id, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (m *Manager) Get(idOrName string) (*Container, error) {
	ids, err := m.store.List()
	if err != nil {
		return nil, err
	}

	

	for _, id := range ids {
		if id == idOrName {
			var c Container
			if err := m.store.Load(id, &c); err != nil {
				return nil, err
			}
			return &c, nil
		}
	}

	

	for _, id := range ids {
		var c Container
		if err := m.store.Load(id, &c); err != nil {
			continue
		}
		for _, n := range c.Names {
			if n == idOrName || n == "/"+idOrName {
				return &c, nil
			}
		}
	}

	

	resolved, err := store.ResolveID(ids, idOrName)
	if err != nil {
		return nil, fmt.Errorf("no such container: %s", idOrName)
	}
	var c Container
	if err := m.store.Load(resolved, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (m *Manager) Save(c *Container) error {
	return m.store.Save(c.ID, c)
}

func (m *Manager) Delete(idOrName string) error {
	c, err := m.Get(idOrName)
	if err != nil {
		return err
	}
	if c.State.Running {
		return fmt.Errorf("cannot remove running container %s; stop it first", idOrName)
	}
	

	containerDir := filepath.Join(config.Global.ContainersDir, c.ID)
	os.RemoveAll(containerDir)
	

	os.Remove(c.LogPath)
	return m.store.Delete(c.ID)
}

func (m *Manager) List(all bool) ([]*Container, error) {
	ids, err := m.store.List()
	if err != nil {
		return nil, err
	}
	var result []*Container
	for _, id := range ids {
		var c Container
		if err := m.store.Load(id, &c); err != nil {
			continue
		}
		if all || c.State.Running {
			result = append(result, &c)
		}
	}
	return result, nil
}

func (m *Manager) ListAll() ([]*Container, error) {
	return m.List(true)
}

func (m *Manager) UpdateState(id string, fn func(*Container)) error {
	c, err := m.Get(id)
	if err != nil {
		return err
	}
	fn(c)
	return m.store.Save(c.ID, c)
}

func (m *Manager) StatusString(c *Container) string {
	switch c.State.Status {
	case StatusRunning:
		dur := time.Since(c.State.StartedAt)
		return fmt.Sprintf("Up %s", formatDuration(dur))
	case StatusExited:
		return fmt.Sprintf("Exited (%d) %s ago", c.State.ExitCode, formatDuration(time.Since(c.State.FinishedAt)))
	case StatusCreated:
		return "Created"
	case StatusStopped:
		return "Stopped"
	case StatusPaused:
		return fmt.Sprintf("Up %s (Paused)", formatDuration(time.Since(c.State.StartedAt)))
	default:
		return string(c.State.Status)
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}

var adjectives = []string{"happy", "clever", "brave", "calm", "eager", "fast", "kind", "proud", "quiet", "swift"}
var nouns = []string{"atlas", "bear", "crane", "drift", "ember", "frost", "grove", "hawk", "iris", "jade"}

func generateName(id string) string {
	a := adjectives[int(id[0])%len(adjectives)]
	n := nouns[int(id[1])%len(nouns)]
	return a + "_" + n
}
