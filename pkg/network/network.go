package network

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/cryruss/cryruss/pkg/config"
	"github.com/cryruss/cryruss/pkg/store"
)

type Network struct {
	ID         string                      `json:"Id"`
	Name       string                      `json:"Name"`
	Created    time.Time                   `json:"Created"`
	Scope      string                      `json:"Scope"`
	Driver     string                      `json:"Driver"`
	EnableIPv6 bool                        `json:"EnableIPv6"`
	IPAM       IPAM                        `json:"IPAM"`
	Internal   bool                        `json:"Internal"`
	Attachable bool                        `json:"Attachable"`
	Labels     map[string]string           `json:"Labels"`
	Options    map[string]string           `json:"Options"`
	Containers map[string]EndpointResource `json:"Containers"`
}

type IPAM struct {
	Driver  string            `json:"Driver"`
	Config  []IPAMConfig      `json:"Config"`
	Options map[string]string `json:"Options"`
}

type IPAMConfig struct {
	Subnet  string `json:"Subnet"`
	Gateway string `json:"Gateway"`
	IPRange string `json:"IPRange,omitempty"`
}

type EndpointResource struct {
	Name        string `json:"Name"`
	EndpointID  string `json:"EndpointID"`
	MacAddress  string `json:"MacAddress"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
}

type CreateRequest struct {
	Name           string            `json:"Name"`
	Driver         string            `json:"Driver"`
	Internal       bool              `json:"Internal"`
	Attachable     bool              `json:"Attachable"`
	EnableIPv6     bool              `json:"EnableIPv6"`
	Labels         map[string]string `json:"Labels"`
	Options        map[string]string `json:"Options"`
	IPAM           *IPAM             `json:"IPAM"`
	CheckDuplicate bool              `json:"CheckDuplicate"`
}

type ConnectRequest struct {
	Container string   `json:"Container"`
	Aliases   []string `json:"Aliases"`
	IPv4      string   `json:"IPv4Address"`
	IPv6      string   `json:"IPv6Address"`
}

type Manager struct {
	store *store.Store
}

func NewManager() *Manager {
	m := &Manager{store: store.New(config.Global.NetworksDir)}
	m.ensureDefaults()
	return m
}

func (m *Manager) ensureDefaults() {
	if !m.store.Exists("host") {
		m.store.Save("host", &Network{
			ID: "host", Name: "host", Created: time.Now(),
			Scope: "local", Driver: "host",
			IPAM:       IPAM{Driver: "default", Config: []IPAMConfig{}, Options: map[string]string{}},
			Labels:     map[string]string{},
			Options:    map[string]string{},
			Containers: map[string]EndpointResource{},
		})
	}
	if !m.store.Exists("none") {
		m.store.Save("none", &Network{
			ID: "none", Name: "none", Created: time.Now(),
			Scope: "local", Driver: "null",
			IPAM:       IPAM{Driver: "default", Config: []IPAMConfig{}, Options: map[string]string{}},
			Labels:     map[string]string{},
			Options:    map[string]string{},
			Containers: map[string]EndpointResource{},
		})
	}
	if !m.store.Exists("bridge") {
		m.store.Save("bridge", &Network{
			ID: "bridge", Name: "bridge", Created: time.Now(),
			Scope: "local", Driver: "bridge",
			IPAM: IPAM{
				Driver:  "default",
				Config:  []IPAMConfig{{Subnet: "172.17.0.0/16", Gateway: "172.17.0.1"}},
				Options: map[string]string{},
			},
			Labels:     map[string]string{},
			Options:    map[string]string{},
			Containers: map[string]EndpointResource{},
		})
	}
}

func (m *Manager) Create(req *CreateRequest) (*Network, error) {
	

	if req.CheckDuplicate || req.Name != "" {
		existing, _ := m.List()
		for _, n := range existing {
			if n.Name == req.Name {
				return nil, fmt.Errorf("network with name %q already exists", req.Name)
			}
		}
	}

	id := store.NewID()
	driver := req.Driver
	if driver == "" {
		driver = "bridge"
	}

	labels := req.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	options := req.Options
	if options == nil {
		options = map[string]string{}
	}

	var ipam IPAM
	if req.IPAM != nil {
		ipam = *req.IPAM
	} else {
		subnet, gateway := allocateSubnet(m)
		ipam = IPAM{
			Driver:  "default",
			Options: map[string]string{},
			Config:  []IPAMConfig{},
		}
		if driver == "bridge" {
			ipam.Config = []IPAMConfig{{Subnet: subnet, Gateway: gateway}}
		}
	}
	if ipam.Options == nil {
		ipam.Options = map[string]string{}
	}

	n := &Network{
		ID:         id,
		Name:       req.Name,
		Created:    time.Now(),
		Scope:      "local",
		Driver:     driver,
		EnableIPv6: req.EnableIPv6,
		Internal:   req.Internal,
		Attachable: req.Attachable,
		Labels:     labels,
		Options:    options,
		IPAM:       ipam,
		Containers: map[string]EndpointResource{},
	}

	if err := m.store.Save(id, n); err != nil {
		return nil, err
	}
	m.store.Save("name-"+req.Name, map[string]string{"id": id})
	return n, nil
}

func (m *Manager) Get(idOrName string) (*Network, error) {
	

	for _, name := range []string{"host", "none", "bridge"} {
		if idOrName == name {
			var n Network
			if err := m.store.Load(name, &n); err != nil {
				return nil, err
			}
			return &n, nil
		}
	}
	

	if m.store.Exists(idOrName) {
		var n Network
		if err := m.store.Load(idOrName, &n); err != nil {
			return nil, err
		}
		return &n, nil
	}
	

	var nameRef map[string]string
	if err := m.store.Load("name-"+idOrName, &nameRef); err == nil {
		if id, ok := nameRef["id"]; ok {
			var n Network
			if err := m.store.Load(id, &n); err == nil {
				return &n, nil
			}
		}
	}
	

	ids, _ := m.store.List()
	for _, id := range ids {
		if len(id) >= len(idOrName) && id[:len(idOrName)] == idOrName {
			var n Network
			if err := m.store.Load(id, &n); err == nil {
				return &n, nil
			}
		}
	}
	return nil, fmt.Errorf("Error: No such network: %s", idOrName)
}

func (m *Manager) List() ([]*Network, error) {
	ids, err := m.store.List()
	if err != nil {
		return nil, err
	}
	var result []*Network
	seen := map[string]bool{}
	

	for _, name := range []string{"bridge", "host", "none"} {
		var n Network
		if err := m.store.Load(name, &n); err == nil && !seen[n.ID] {
			seen[n.ID] = true
			result = append(result, &n)
		}
	}
	for _, id := range ids {
		if id == "host" || id == "none" || id == "bridge" {
			continue
		}
		if len(id) > 5 && id[:5] == "name-" {
			continue
		}
		var n Network
		if err := m.store.Load(id, &n); err == nil && !seen[n.ID] {
			seen[n.ID] = true
			result = append(result, &n)
		}
	}
	return result, nil
}

func (m *Manager) Delete(idOrName string) error {
	return m.DeleteForce(idOrName, false)
}

func (m *Manager) DeleteForce(idOrName string, force bool) error {
	n, err := m.Get(idOrName)
	if err != nil {
		return err
	}
	if n.Name == "host" || n.Name == "none" || n.Name == "bridge" {
		return fmt.Errorf("Error response from daemon: cannot delete built-in network: %s", n.Name)
	}
	if !force && len(n.Containers) > 0 {
		return fmt.Errorf("Error response from daemon: network %s id %s has active endpoints", n.Name, n.ID[:12])
	}
	m.store.Delete("name-" + n.Name)
	return m.store.Delete(n.ID)
}

func (m *Manager) Prune() ([]string, error) {
	networks, err := m.List()
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, n := range networks {
		if n.Name == "host" || n.Name == "none" || n.Name == "bridge" {
			continue
		}
		if len(n.Containers) == 0 {
			if err := m.Delete(n.ID); err == nil {
				pruned = append(pruned, n.Name)
			}
		}
	}
	return pruned, nil
}

func (m *Manager) Connect(networkIDOrName, containerID, containerName string, req *ConnectRequest) error {
	n, err := m.Get(networkIDOrName)
	if err != nil {
		return err
	}
	if n.Containers == nil {
		n.Containers = map[string]EndpointResource{}
	}
	if _, already := n.Containers[containerID]; already {
		return fmt.Errorf("Error response from daemon: endpoint with name %s already exists in network %s", containerName, n.Name)
	}

	ip := req.IPv4
	if ip == "" && len(n.IPAM.Config) > 0 {
		ip = allocateContainerIP(n)
	}

	ep := EndpointResource{
		Name:        containerName,
		EndpointID:  store.NewID()[:32],
		MacAddress:  randomMAC(),
		IPv4Address: ip,
		IPv6Address: req.IPv6,
	}
	n.Containers[containerID] = ep

	key := n.ID
	if n.Name == "host" || n.Name == "none" || n.Name == "bridge" {
		key = n.Name
	}
	return m.store.Save(key, n)
}

func (m *Manager) Disconnect(networkIDOrName, containerID string, force bool) error {
	n, err := m.Get(networkIDOrName)
	if err != nil {
		return err
	}
	if _, ok := n.Containers[containerID]; !ok {
		if !force {
			return fmt.Errorf("Error response from daemon: container %s is not connected to network %s", containerID[:12], n.Name)
		}
		return nil
	}
	delete(n.Containers, containerID)
	key := n.ID
	if n.Name == "host" || n.Name == "none" || n.Name == "bridge" {
		key = n.Name
	}
	return m.store.Save(key, n)
}

func allocateSubnet(m *Manager) (subnet, gateway string) {
	used := map[string]bool{}
	networks, _ := m.List()
	for _, n := range networks {
		for _, cfg := range n.IPAM.Config {
			used[cfg.Subnet] = true
		}
	}
	for b := 18; b <= 31; b++ {
		s := fmt.Sprintf("172.%d.0.0/16", b)
		if !used[s] {
			return s, fmt.Sprintf("172.%d.0.1", b)
		}
	}
	

	rnd := 100 + rand.Intn(100)
	return fmt.Sprintf("10.%d.0.0/24", rnd), fmt.Sprintf("10.%d.0.1", rnd)
}

func allocateContainerIP(n *Network) string {
	if len(n.IPAM.Config) == 0 {
		return ""
	}
	_, ipNet, err := net.ParseCIDR(n.IPAM.Config[0].Subnet)
	if err != nil {
		return ""
	}
	used := map[string]bool{}
	for _, ep := range n.Containers {
		used[ep.IPv4Address] = true
	}
	ip := cloneIP(ipNet.IP)
	inc(ip) 

	inc(ip) 

	inc(ip)
	for ipNet.Contains(ip) {
		s := ip.String()
		if !used[s] {
			return s + "/" + maskBits(ipNet)
		}
		inc(ip)
	}
	return ""
}

func cloneIP(ip net.IP) net.IP {
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func maskBits(n *net.IPNet) string {
	ones, _ := n.Mask.Size()
	return fmt.Sprintf("%d", ones)
}

func randomMAC() string {
	b := make([]byte, 6)
	rand.Read(b)
	b[0] = (b[0] | 0x02) & 0xfe 

	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
}
