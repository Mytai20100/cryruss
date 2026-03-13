package container

import "time"

type Status string

const (
	StatusCreated  Status = "created"
	StatusRunning  Status = "running"
	StatusStopped  Status = "stopped"
	StatusPaused   Status = "paused"
	StatusRemoving Status = "removing"
	StatusExited   Status = "exited"
	StatusDead     Status = "dead"
)

type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type PortMap map[string][]PortBinding

type Mount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
	Propagation string `json:"Propagation"`
}

type HostConfig struct {
	Binds         []string      `json:"Binds"`
	PortBindings  PortMap       `json:"PortBindings"`
	NetworkMode   string        `json:"NetworkMode"`
	RestartPolicy RestartPolicy `json:"RestartPolicy"`
	AutoRemove    bool          `json:"AutoRemove"`
	// Memory limits
	Memory            int64 `json:"Memory"`            // bytes
	MemorySwap        int64 `json:"MemorySwap"`        // bytes, -1 = unlimited
	MemorySwappiness  int64 `json:"MemorySwappiness"`  // 0-100, -1 = default
	MemoryReservation int64 `json:"MemoryReservation"` // soft limit bytes
	KernelMemory      int64 `json:"KernelMemory"`      // kernel memory limit bytes
	// CPU limits
	NanoCPUs    int64  `json:"NanoCPUs"`   // e.g. 1.5 CPUs = 1500000000
	CPUShares   int64  `json:"CpuShares"`  // relative weight 10-1024
	CPUPeriod   int64  `json:"CpuPeriod"`  // microseconds, default 100000
	CPUQuota    int64  `json:"CpuQuota"`   // microseconds per period
	CPUSetCPUs  string `json:"CpusetCpus"` // e.g. "0-3" or "0,1"
	CPUSetMems  string `json:"CpusetMems"` // NUMA nodes
	CPURealtime int64  `json:"CpuRealtimePeriod"`
	// Block I/O limits
	BlkioWeight          uint16              `json:"BlkioWeight"` // 10-1000
	BlkioWeightDevice    []BlkioWeightDevice `json:"BlkioWeightDevice"`
	BlkioDeviceReadBps   []ThrottleDevice    `json:"BlkioDeviceReadBps"`
	BlkioDeviceWriteBps  []ThrottleDevice    `json:"BlkioDeviceWriteBps"`
	BlkioDeviceReadIOps  []ThrottleDevice    `json:"BlkioDeviceReadIOps"`
	BlkioDeviceWriteIOps []ThrottleDevice    `json:"BlkioDeviceWriteIOps"`
	// Disk / storage
	StorageOpt map[string]string `json:"StorageOpt"` // e.g. {"size":"10G"}
	// Other resource limits
	PidsLimit      int64  `json:"PidsLimit"`      // max processes, -1 = unlimited
	SeccompProfile string `json:"SeccompProfile"` // path or "unconfined"
	// Existing fields
	SecurityOpt     []string          `json:"SecurityOpt"`
	CapAdd          []string          `json:"CapAdd"`
	CapDrop         []string          `json:"CapDrop"`
	ReadonlyRootfs  bool              `json:"ReadonlyRootfs"`
	Tmpfs           map[string]string `json:"Tmpfs"`
	ShmSize         int64             `json:"ShmSize"`
	UsernsMode      string            `json:"UsernsMode"`
	PidMode         string            `json:"PidMode"`
	IpcMode         string            `json:"IpcMode"`
	UTSMode         string            `json:"UTSMode"`
	Init            bool              `json:"Init"`
	ExtraHosts      []string          `json:"ExtraHosts"`
	DNS             []string          `json:"Dns"`
	DNSSearch       []string          `json:"DnsSearch"`
	DNSOptions      []string          `json:"DnsOptions"`
	Devices         []string          `json:"Devices"`
	LogConfig       LogConfig         `json:"LogConfig"`
	Ulimits         []Ulimit          `json:"Ulimits"`
	Links           []string          `json:"Links"`
	Privileged      bool              `json:"Privileged"`
	PublishAllPorts bool              `json:"PublishAllPorts"`
	NetworkAliases  []string          `json:"NetworkAliases"`
}

// BlkioWeightDevice sets per-device blkio weight.
type BlkioWeightDevice struct {
	Path   string `json:"Path"`
	Weight uint16 `json:"Weight"`
}

// ThrottleDevice sets a per-device I/O rate limit.
type ThrottleDevice struct {
	Path string `json:"Path"`
	Rate int64  `json:"Rate"` // bytes/s or IOPS depending on field
}

type LogConfig struct {
	Type   string            `json:"Type"`
	Config map[string]string `json:"Config"`
}

type Ulimit struct {
	Name string `json:"Name"`
	Soft int64  `json:"Soft"`
	Hard int64  `json:"Hard"`
}

type RestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

type NetworkSettings struct {
	Networks  map[string]*EndpointSettings `json:"Networks"`
	Ports     PortMap                      `json:"Ports"`
	IPAddress string                       `json:"IPAddress"`
	Gateway   string                       `json:"Gateway"`
}

type EndpointSettings struct {
	NetworkID  string `json:"NetworkID"`
	EndpointID string `json:"EndpointID"`
	Gateway    string `json:"Gateway"`
	IPAddress  string `json:"IPAddress"`
}

type State struct {
	Status     Status    `json:"Status"`
	Running    bool      `json:"Running"`
	Paused     bool      `json:"Paused"`
	Restarting bool      `json:"Restarting"`
	Dead       bool      `json:"Dead"`
	Pid        int       `json:"Pid"`
	ExitCode   int       `json:"ExitCode"`
	Error      string    `json:"Error"`
	StartedAt  time.Time `json:"StartedAt"`
	FinishedAt time.Time `json:"FinishedAt"`
}

type Config struct {
	Hostname     string              `json:"Hostname"`
	Domainname   string              `json:"Domainname"`
	User         string              `json:"User"`
	AttachStdin  bool                `json:"AttachStdin"`
	AttachStdout bool                `json:"AttachStdout"`
	AttachStderr bool                `json:"AttachStderr"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Tty          bool                `json:"Tty"`
	OpenStdin    bool                `json:"OpenStdin"`
	Env          []string            `json:"Env"`
	Cmd          []string            `json:"Cmd"`
	Image        string              `json:"Image"`
	Volumes      map[string]struct{} `json:"Volumes"`
	WorkingDir   string              `json:"WorkingDir"`
	Entrypoint   []string            `json:"Entrypoint"`
	Labels       map[string]string   `json:"Labels"`
	StopSignal   string              `json:"StopSignal"`
	StopTimeout  int                 `json:"StopTimeout"`
}

type Container struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	ImageID         string            `json:"ImageID"`
	Command         string            `json:"Command"`
	Created         int64             `json:"Created"`
	Status          string            `json:"Status"`
	State           State             `json:"State"`
	Ports           []PortInfo        `json:"Ports"`
	Labels          map[string]string `json:"Labels"`
	Mounts          []Mount           `json:"Mounts"`
	HostConfig      HostConfig        `json:"HostConfig"`
	NetworkSettings NetworkSettings   `json:"NetworkSettings"`
	Config          Config            `json:"Config"`
	RootfsPath      string            `json:"RootfsPath"`
	LogPath         string            `json:"LogPath"`
}

type PortInfo struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type CreateRequest struct {
	Image        string              `json:"Image"`
	ImageID      string              `json:"ImageID"`
	Cmd          []string            `json:"Cmd"`
	Entrypoint   []string            `json:"Entrypoint"`
	Env          []string            `json:"Env"`
	Labels       map[string]string   `json:"Labels"`
	Hostname     string              `json:"Hostname"`
	User         string              `json:"User"`
	WorkingDir   string              `json:"WorkingDir"`
	Tty          bool                `json:"Tty"`
	OpenStdin    bool                `json:"OpenStdin"`
	AttachStdin  bool                `json:"AttachStdin"`
	AttachStdout bool                `json:"AttachStdout"`
	AttachStderr bool                `json:"AttachStderr"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Volumes      map[string]struct{} `json:"Volumes"`
	StopSignal   string              `json:"StopSignal"`
	StopTimeout  int                 `json:"StopTimeout"`
	HostConfig   HostConfig          `json:"HostConfig"`
}
