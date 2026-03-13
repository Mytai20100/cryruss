package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	cryruss "github.com/cryruss/cryruss"
	"github.com/cryruss/cryruss/pkg/api"
	"github.com/cryruss/cryruss/pkg/config"
	"github.com/cryruss/cryruss/pkg/container"
	"github.com/cryruss/cryruss/pkg/image"
	"github.com/cryruss/cryruss/pkg/network"
	rt "github.com/cryruss/cryruss/pkg/runtime"
	"github.com/cryruss/cryruss/pkg/volume"
)

func main() {
	

	if len(os.Args) > 1 && os.Args[1] == rt.InitArg {
		rt.ContainerInit()
		os.Exit(0)
	}

	config.Init()

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	

	os.Args = expandCombinedFlags(os.Args)

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "run":
		cmdRun(args)
	case "start":
		cmdStart(args)
	case "stop":
		cmdStop(args)
	case "restart":
		cmdRestart(args)
	case "rm":
		cmdRm(args)
	case "ps":
		cmdPs(args)
	case "create":
		cmdCreate(args)
	case "exec":
		cmdExec(args)
	case "logs":
		cmdLogs(args)
	case "inspect":
		cmdInspect(args)
	case "kill":
		cmdKill(args)
	case "pause":
		cmdPause(args)
	case "unpause":
		cmdUnpause(args)
	case "rename":
		cmdRename(args)
	case "stats":
		cmdStats(args)
	case "top":
		cmdTop(args)
	case "wait":
		cmdWait(args)
	case "port":
		cmdPort(args)
	case "diff":
		cmdDiff(args)
	case "cp":
		cmdCp(args)
	case "images", "image":
		if len(args) > 0 {
			switch args[0] {
			case "ls", "list":
				cmdImages(args[1:])
			case "rm", "remove":
				cmdRmi(args[1:])
			case "pull":
				cmdPull(args[1:])
			case "tag":
				cmdTag(args[1:])
			case "inspect":
				cmdImageInspect(args[1:])
			case "prune":
				cmdImagePrune(args[1:])
			case "history":
				cmdImageHistory(args[1:])
			default:
				cmdImages(args)
			}
		} else {
			cmdImages(args)
		}
	case "pull":
		cmdPull(args)
	case "rmi":
		cmdRmi(args)
	case "tag":
		cmdTag(args)
	case "network":
		cmdNetwork(args)
	case "stack":
		cmdStack(args)
	case "volume":
		cmdVolume(args)
	case "system":
		cmdSystem(args)
	case "info":
		cmdInfo(args)
	case "version":
		cmdVersion(args)
	case "serve", "api":
		cmdServe(args)
	case "help", "--help", "-h":
		usage()
	case "--version", "-v":
		fmt.Printf("cryruss version %s\n", cryruss.Version)
	case "prune":
		cmdSystemPrune(args)
	default:
		fmt.Fprintf(os.Stderr, "cryruss: unknown command: %s\n", cmd)
		fmt.Fprintf(os.Stderr, "run 'cryruss help' for usage\n")
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`Usage: cryruss [OPTIONS] COMMAND

Non-root container runtime

Commands:
  run        Run a command in a new container
  create     Create a new container
  start      Start one or more stopped containers
  stop       Stop one or more running containers
  restart    Restart one or more containers
  rm         Remove one or more containers
  ps         List containers
  exec       Run a command in a running container
  logs       Fetch the logs of a container
  inspect    Return low-level information on containers/images
  kill       Kill one or more running containers
  pause      Pause all processes within one or more containers
  unpause    Unpause all processes within one or more containers
  rename     Rename a container
  stats      Display a live stream of container resource usage statistics
  top        Display the running processes of a container
  wait       Block until one or more containers stop, then print exit codes
  port       List port mappings or a specific mapping for a container
  diff       Inspect changes to files on a container's filesystem
  cp         Copy files/folders between a container and the local filesystem

  pull       Pull an image from a registry
  images     List images
  rmi        Remove one or more images
  tag        Create a tag pointing to an image
  image      Manage images

  network    Manage networks
  stack      Manage Stacks
  volume     Manage volumes
  system     Manage Cryruss
  info       Display system-wide information
  version    Show the Cryruss version information
  serve      Start the API server (unix socket)

Run 'cryruss COMMAND --help' for more information on a command.
`)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	detach := fs.Bool("d", false, "Run container in background")
	fs.Bool("detach", false, "Run container in background")
	interactive := fs.Bool("i", false, "Keep STDIN open")
	tty := fs.Bool("t", false, "Allocate a pseudo-TTY")
	rm := fs.Bool("rm", false, "Automatically remove container when it exits")
	name := fs.String("name", "", "Assign a name to the container")
	hostname := fs.String("hostname", "", "Container host name")
	hostname2 := fs.String("h", "", "Container host name")
	workdir := fs.String("w", "", "Working directory inside the container")
	workdir2 := fs.String("workdir", "", "Working directory inside the container")
	user := fs.String("u", "", "Username or UID")
	user2 := fs.String("user", "", "Username or UID")
	netMode := fs.String("network", "host", "Connect a container to a network")
	label := fs.String("label", "", "Set metadata on container")
	entrypoint := fs.String("entrypoint", "", "Overwrite the default ENTRYPOINT")
	stopSignal := fs.String("stop-signal", "SIGTERM", "Signal to stop a container")
	stopTimeout := fs.Int("stop-timeout", 10, "Timeout (in seconds) to stop a container")
	init := fs.Bool("init", false, "Run an init inside the container")
	readOnly := fs.Bool("read-only", false, "Mount container's root filesystem as read only")
	useProot := fs.Bool("proot", false, "Use proot instead of user namespaces (for non-root envs without newuidmap)")

	

	memory := fs.String("memory", "", "Memory limit (e.g. 512m, 1g)")
	fs.String("m", "", "Memory limit (shorthand)")
	memorySwap := fs.String("memory-swap", "", "Swap limit (memory+swap, -1 = unlimited)")
	memorySwappiness := fs.Int64("memory-swappiness", -1, "Tune memory swappiness (0-100, -1=default)")
	memoryReservation := fs.String("memory-reservation", "", "Memory soft limit")
	kernelMemory := fs.String("kernel-memory", "", "Kernel memory limit")

	

	cpus := fs.Float64("cpus", 0, "Number of CPUs (e.g. 1.5)")
	cpuShares := fs.Int64("cpu-shares", 0, "CPU shares (relative weight, default 1024)")
	fs.Int64("c", 0, "CPU shares shorthand")
	cpuPeriod := fs.Int64("cpu-period", 0, "Limit CPU CFS period (microseconds)")
	cpuQuota := fs.Int64("cpu-quota", 0, "Limit CPU CFS quota (microseconds per period)")
	cpusetCpus := fs.String("cpuset-cpus", "", "CPUs to allow (e.g. 0-3, 0,1)")
	cpusetMems := fs.String("cpuset-mems", "", "NUMA nodes to allow")

	

	blkioWeight := fs.Uint64("blkio-weight", 0, "Block IO weight (10-1000)")
	var blkioWeightDevice multiFlag
	fs.Var(&blkioWeightDevice, "blkio-weight-device", "Block IO weight per device (PATH:WEIGHT)")
	var deviceReadBps multiFlag
	fs.Var(&deviceReadBps, "device-read-bps", "Limit read rate from device (PATH:RATE, e.g. /dev/sda:10mb)")
	var deviceWriteBps multiFlag
	fs.Var(&deviceWriteBps, "device-write-bps", "Limit write rate to device (PATH:RATE)")
	var deviceReadIops multiFlag
	fs.Var(&deviceReadIops, "device-read-iops", "Limit read IOPS from device (PATH:IOPS)")
	var deviceWriteIops multiFlag
	fs.Var(&deviceWriteIops, "device-write-iops", "Limit write IOPS to device (PATH:IOPS)")

	

	pidsLimit := fs.Int64("pids-limit", 0, "Tune container pids limit (-1 = unlimited)")

	

	var storageOpts multiFlag
	fs.Var(&storageOpts, "storage-opt", "Storage driver options (e.g. size=10G)")

	

	var envs multiFlag
	fs.Var(&envs, "e", "Set environment variables")
	fs.Var(&envs, "env", "Set environment variables")

	var volumes multiFlag
	fs.Var(&volumes, "v", "Bind mount a volume")
	fs.Var(&volumes, "volume", "Bind mount a volume")

	var ports multiFlag
	fs.Var(&ports, "p", "Publish container port(s)")
	fs.Var(&ports, "publish", "Publish container port(s)")

	var capAdd multiFlag
	fs.Var(&capAdd, "cap-add", "Add Linux capabilities")

	var capDrop multiFlag
	fs.Var(&capDrop, "cap-drop", "Drop Linux capabilities")

	var secOpts multiFlag
	fs.Var(&secOpts, "security-opt", "Security options")

	

	var addHosts multiFlag
	fs.Var(&addHosts, "add-host", "Add a custom host-to-IP mapping (host:ip)")
	var dns multiFlag
	fs.Var(&dns, "dns", "Set custom DNS servers")
	var dnsSearch multiFlag
	fs.Var(&dnsSearch, "dns-search", "Set custom DNS search domains")
	var dnsOpt multiFlag
	fs.Var(&dnsOpt, "dns-option", "Set DNS options")
	var netAliases multiFlag
	fs.Var(&netAliases, "network-alias", "Add network-scoped alias for the container")
	var devices multiFlag
	fs.Var(&devices, "device", "Add a host device to the container")
	var links multiFlag
	fs.Var(&links, "link", "Add link to another container")

	

	privileged := fs.Bool("privileged", false, "Give extended privileges to this container")
	publishAll := fs.Bool("P", false, "Publish all exposed ports to random ports")
	restart := fs.String("restart", "no", "Restart policy (no|always|on-failure|unless-stopped)")
	pid := fs.String("pid", "", "PID namespace to use")
	ipc := fs.String("ipc", "", "IPC mode to use")
	uts := fs.String("uts", "", "UTS namespace to use")
	cgroupns := fs.String("cgroupns", "", "Cgroup namespace to use")
	shmSize := fs.String("shm-size", "", "Size of /dev/shm")
	macAddress := fs.String("mac-address", "", "Container MAC address")
	ip := fs.String("ip", "", "IPv4 address (e.g., 172.30.100.104)")

	

	logDriver := fs.String("log-driver", "json-file", "Logging driver for the container")
	var logOpts multiFlag
	fs.Var(&logOpts, "log-opt", "Log driver options")

	

	var envFile multiFlag
	fs.Var(&envFile, "env-file", "Read in a file of environment variables")

	

	var ulimits multiFlag
	fs.Var(&ulimits, "ulimit", "Ulimit options (e.g. nofile=1024:2048)")

	fs.Usage = func() {
		fmt.Print("Usage: cryruss run [OPTIONS] IMAGE [COMMAND] [ARG...]\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "cryruss run: IMAGE required")
		os.Exit(1)
	}
	imageName := remaining[0]
	cmdArgs := remaining[1:]

	wdir := *workdir
	if *workdir2 != "" {
		wdir = *workdir2
	}
	usr := *user
	if *user2 != "" {
		usr = *user2
	}
	hname := *hostname
	if *hostname2 != "" {
		hname = *hostname2
	}

	portMap := parsePortBindings(ports)
	mounts := parseMounts(volumes)

	

	mem := parseMemory(*memory)
	memSwap := parseMemory(*memorySwap)
	memRes := parseMemory(*memoryReservation)
	kernMem := parseMemory(*kernelMemory)
	nanoCPUs := int64(*cpus * 1e9)

	req := &container.CreateRequest{
		Image:        imageName,
		Cmd:          cmdArgs,
		Env:          envs,
		Hostname:     hname,
		User:         usr,
		WorkingDir:   wdir,
		Tty:          *tty,
		OpenStdin:    *interactive,
		AttachStdin:  *interactive,
		AttachStdout: true,
		AttachStderr: true,
		StopSignal:   *stopSignal,
		StopTimeout:  *stopTimeout,
		Labels:       parseLabels(*label),
		HostConfig: container.HostConfig{
			Binds:        mounts,
			PortBindings: portMap,
			NetworkMode:  *netMode,
			AutoRemove:   *rm,
			

			Memory:            mem,
			MemorySwap:        memSwap,
			MemorySwappiness:  *memorySwappiness,
			MemoryReservation: memRes,
			KernelMemory:      kernMem,
			

			NanoCPUs:   nanoCPUs,
			CPUShares:  *cpuShares,
			CPUPeriod:  *cpuPeriod,
			CPUQuota:   *cpuQuota,
			CPUSetCPUs: *cpusetCpus,
			CPUSetMems: *cpusetMems,
			

			BlkioWeight:          uint16(*blkioWeight),
			BlkioWeightDevice:    parseBlkioWeightDevices(blkioWeightDevice),
			BlkioDeviceReadBps:   parseBlkioThrottleDevices(deviceReadBps),
			BlkioDeviceWriteBps:  parseBlkioThrottleDevices(deviceWriteBps),
			BlkioDeviceReadIOps:  parseBlkioThrottleDevices(deviceReadIops),
			BlkioDeviceWriteIOps: parseBlkioThrottleDevices(deviceWriteIops),
			

			PidsLimit: *pidsLimit,
			

			StorageOpt: parseKeyValueList(storageOpts),
			

			CapAdd:          capAdd,
			CapDrop:         capDrop,
			SecurityOpt:     secOpts,
			ReadonlyRootfs:  *readOnly,
			Init:            *init,
			ExtraHosts:      []string(addHosts),
			DNS:             []string(dns),
			DNSSearch:       []string(dnsSearch),
			DNSOptions:      []string(dnsOpt),
			Devices:         []string(devices),
			Links:           []string(links),
			NetworkAliases:  []string(netAliases),
			Privileged:      *privileged,
			PublishAllPorts: *publishAll,
			RestartPolicy:   container.RestartPolicy{Name: *restart},
			LogConfig:       container.LogConfig{Type: *logDriver, Config: parseKeyValueList(logOpts)},
			Ulimits:         parseUlimits(ulimits),
		},
	}
	if *entrypoint != "" {
		req.Entrypoint = strings.Fields(*entrypoint)
	}
	

	for _, ef := range envFile {
		lines, err := readEnvFile(ef)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: env-file %s: %v\n", ef, err)
			os.Exit(1)
		}
		req.Env = append(lines, req.Env...)
	}
	_ = *macAddress
	_ = *ip
	_ = *pid
	_ = *ipc
	_ = *uts
	_ = *cgroupns
	_ = *shmSize

	mgr := container.NewManager()
	imgMgr := image.NewManager()

	

	img, err := imgMgr.Get(imageName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pulling %s...\n", imageName)
		img, err = imgMgr.Pull(imageName, func(p image.PullProgress) {
			if p.Done {
				fmt.Fprintf(os.Stderr, "%s: Pull complete\n", p.Layer)
			}
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: pull: %v\n", err)
			os.Exit(1)
		}
	}

	

	if img.Config != nil {
		if len(req.Cmd) == 0 && len(req.Entrypoint) == 0 {
			req.Cmd = img.Config.Cmd
			req.Entrypoint = img.Config.Entrypoint
		}
		if req.WorkingDir == "" && img.Config.WorkingDir != "" {
			req.WorkingDir = img.Config.WorkingDir
		}
		if req.User == "" && img.Config.User != "" {
			req.User = img.Config.User
		}
		if img.Config.StopSignal != "" && req.StopSignal == "SIGTERM" {
			req.StopSignal = img.Config.StopSignal
		}
		req.Env = mergeEnv(img.Config.Env, req.Env)
	}
	req.ImageID = img.ID

	c, err := mgr.Create(req, *name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: create: %v\n", err)
		os.Exit(1)
	}

	

	if img != nil {
		if err := imgMgr.PrepareRootfs(img, c.RootfsPath); err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: rootfs: %v\n", err)
			os.Exit(1)
		}
	}

	opts := rt.RunOptions{
		Detach:      *detach,
		Interactive: *interactive,
		TTY:         *tty,
	}

	

	startFn := rt.Start
	if *useProot {
		startFn = rt.StartWithProot
	}

	if *detach {
		proc, err := startFn(c, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: start: %v\n", err)
			os.Exit(1)
		}
		mgr.UpdateState(c.ID, func(c *container.Container) {
			c.State.Status = container.StatusRunning
			c.State.Running = true
			c.State.Pid = proc.Pid
			c.State.StartedAt = time.Now()
		})
		fmt.Println(c.ID)

		go func() {
			proc.Wait()
			mgr.UpdateState(c.ID, func(c *container.Container) {
				c.State.Running = false
				c.State.Status = container.StatusExited
				c.State.FinishedAt = time.Now()
				c.State.Pid = 0
			})
			if *rm {
				mgr.Delete(c.ID)
			}
		}()
	} else {
		proc, err := startFn(c, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: start: %v\n", err)
			os.Exit(1)
		}
		mgr.UpdateState(c.ID, func(c *container.Container) {
			c.State.Status = container.StatusRunning
			c.State.Running = true
			c.State.Pid = proc.Pid
			c.State.StartedAt = time.Now()
		})

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sig
			proc.Signal(syscall.SIGTERM)
		}()

		state, _ := proc.Wait()
		exitCode := 0
		if state != nil {
			exitCode = state.ExitCode()
		}
		mgr.UpdateState(c.ID, func(c *container.Container) {
			c.State.Running = false
			c.State.Status = container.StatusExited
			c.State.ExitCode = exitCode
			c.State.FinishedAt = time.Now()
			c.State.Pid = 0
		})
		if *rm {
			mgr.Delete(c.ID)
		}
		os.Exit(exitCode)
	}
}

func cmdCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	name := fs.String("name", "", "Assign a name to the container")
	hostname := fs.String("hostname", "", "Container host name")
	workdir := fs.String("w", "", "Working directory")
	user := fs.String("u", "", "Username or UID")
	netMode := fs.String("network", "host", "Network mode")
	entrypoint := fs.String("entrypoint", "", "Override entrypoint")
	stopSignal := fs.String("stop-signal", "SIGTERM", "Signal to stop")
	stopTimeout := fs.Int("stop-timeout", 10, "Stop timeout")
	var envs, volumes, ports, capAdd, capDrop multiFlag
	fs.Var(&envs, "e", "Environment variables")
	fs.Var(&envs, "env", "Environment variables")
	fs.Var(&volumes, "v", "Volumes")
	fs.Var(&ports, "p", "Ports")
	fs.Var(&capAdd, "cap-add", "Add capabilities")
	fs.Var(&capDrop, "cap-drop", "Drop capabilities")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "IMAGE required")
		os.Exit(1)
	}

	req := &container.CreateRequest{
		Image:       remaining[0],
		Cmd:         remaining[1:],
		Env:         envs,
		Hostname:    *hostname,
		User:        *user,
		WorkingDir:  *workdir,
		StopSignal:  *stopSignal,
		StopTimeout: *stopTimeout,
		HostConfig: container.HostConfig{
			Binds:        parseMounts(volumes),
			PortBindings: parsePortBindings(ports),
			NetworkMode:  *netMode,
			CapAdd:       capAdd,
			CapDrop:      capDrop,
		},
	}
	if *entrypoint != "" {
		req.Entrypoint = strings.Fields(*entrypoint)
	}

	mgr := container.NewManager()
	imgMgr := image.NewManager()
	img, err := imgMgr.Get(remaining[0])
	if err == nil && img.Config != nil {
		if len(req.Cmd) == 0 {
			req.Cmd = img.Config.Cmd
			req.Entrypoint = img.Config.Entrypoint
		}
		req.Env = mergeEnv(img.Config.Env, req.Env)
	}

	c, err := mgr.Create(req, *name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(c.ID)
}

func cmdPs(args []string) {
	fs := flag.NewFlagSet("ps", flag.ExitOnError)
	all := fs.Bool("a", false, "Show all containers")
	all2 := fs.Bool("all", false, "Show all containers")
	quiet := fs.Bool("q", false, "Only display container IDs")
	noTrunc := fs.Bool("no-trunc", false, "Don't truncate output")
	format := fs.String("format", "", "Format output using a Go template")
	fs.Parse(args)

	showAll := *all || *all2
	mgr := container.NewManager()
	containers, err := mgr.List(showAll)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}

	if *quiet {
		for _, c := range containers {
			if *noTrunc {
				fmt.Println(c.ID)
			} else {
				fmt.Println(c.ID[:12])
			}
		}
		return
	}

	if *format != "" {
		for _, c := range containers {
			out := *format
			out = strings.ReplaceAll(out, "{{.ID}}", c.ID[:12])
			out = strings.ReplaceAll(out, "{{.Names}}", strings.Join(c.Names, ","))
			out = strings.ReplaceAll(out, "{{.Image}}", c.Image)
			out = strings.ReplaceAll(out, "{{.Status}}", mgr.StatusString(c))
			fmt.Println(out)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")
	for _, c := range containers {
		id := c.ID[:12]
		if *noTrunc {
			id = c.ID
		}
		cmd := c.Command
		if cmd == "" && len(c.Config.Cmd) > 0 {
			cmd = strings.Join(c.Config.Cmd, " ")
		}
		if len(cmd) > 20 && !*noTrunc {
			cmd = `"` + cmd[:17] + `..."`
		} else if cmd != "" {
			cmd = `"` + cmd + `"`
		}
		created := time.Since(time.Unix(c.Created, 0))
		fmt.Fprintf(w, "%s\t%s\t%s\t%s ago\t%s\t%s\t%s\n",
			id,
			c.Image,
			cmd,
			formatDur(created),
			mgr.StatusString(c),
			formatPorts(c),
			strings.Join(c.Names, ", "),
		)
	}
	w.Flush()
}

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	interactive := fs.Bool("i", false, "Attach container's STDIN")
	attach := fs.Bool("a", false, "Attach STDOUT/STDERR")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "container ID required")
		os.Exit(1)
	}

	mgr := container.NewManager()
	imgMgr := image.NewManager()

	for _, id := range fs.Args() {
		c, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		if c.State.Running {
			fmt.Println(id)
			continue
		}
		if !hasRootfs(c.RootfsPath) {
			img, err := imgMgr.Get(c.Config.Image)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cryruss: image not found: %v\n", err)
				continue
			}
			imgMgr.PrepareRootfs(img, c.RootfsPath)
		}
		opts := rt.RunOptions{
			Detach:      !*attach,
			Interactive: *interactive,
		}
		proc, err := rt.Start(c, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		mgr.UpdateState(c.ID, func(c *container.Container) {
			c.State.Running = true
			c.State.Status = container.StatusRunning
			c.State.Pid = proc.Pid
			c.State.StartedAt = time.Now()
		})
		go func(cid string, p *os.Process) {
			p.Wait()
			mgr.UpdateState(cid, func(c *container.Container) {
				c.State.Running = false
				c.State.Status = container.StatusExited
				c.State.FinishedAt = time.Now()
			})
		}(c.ID, proc)
		fmt.Println(c.ID)
	}
}

func cmdStop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	timeout := fs.Int("t", 10, "Seconds to wait before killing")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "container ID required")
		os.Exit(1)
	}

	mgr := container.NewManager()
	for _, id := range fs.Args() {
		c, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		rt.Stop(c, syscall.SIGTERM, *timeout)
		mgr.UpdateState(c.ID, func(c *container.Container) {
			c.State.Running = false
			c.State.Status = container.StatusStopped
			c.State.FinishedAt = time.Now()
			c.State.Pid = 0
		})
		fmt.Println(c.ID)
	}
}

func cmdRestart(args []string) {
	fs := flag.NewFlagSet("restart", flag.ExitOnError)
	timeout := fs.Int("t", 10, "Seconds to wait before killing")
	fs.Parse(args)
	_ = timeout
	for _, id := range fs.Args() {
		cmdStop([]string{"-t", strconv.Itoa(*timeout), id})
		cmdStart([]string{id})
	}
}

func cmdRm(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	force := fs.Bool("f", false, "Force removal of running container")
	volumes := fs.Bool("v", false, "Remove volumes")
	fs.Parse(args)
	_ = volumes

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "container ID required")
		os.Exit(1)
	}

	mgr := container.NewManager()
	for _, id := range fs.Args() {
		c, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		if c.State.Running {
			if !*force {
				fmt.Fprintf(os.Stderr, "cryruss: container is running; stop it or use -f\n")
				continue
			}
			rt.Stop(c, syscall.SIGKILL, 5)
		}
		if err := mgr.Delete(id); err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		fmt.Println(id)
	}
}

func cmdExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	interactive := fs.Bool("i", false, "Keep STDIN open")
	tty := fs.Bool("t", false, "Allocate pseudo-TTY")
	detach := fs.Bool("d", false, "Run in background")
	workdir := fs.String("w", "", "Working directory")
	user := fs.String("u", "", "User")
	var envs multiFlag
	fs.Var(&envs, "e", "Environment variables")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cryruss exec [OPTIONS] CONTAINER COMMAND [ARG...]")
		os.Exit(1)
	}

	mgr := container.NewManager()
	c, err := mgr.Get(remaining[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	if !c.State.Running {
		fmt.Fprintf(os.Stderr, "cryruss: container is not running\n")
		os.Exit(1)
	}

	

	pid := rt.GetPID(c.ID)
	if pid <= 0 {
		pid = c.State.Pid
	}
	if pid <= 0 {
		fmt.Fprintf(os.Stderr, "cryruss: cannot find container PID\n")
		os.Exit(1)
	}

	nsenterArgs := []string{
		fmt.Sprintf("--target=%d", pid),
		"--mount", "--uts", "--ipc", "--pid",
	}
	if c.HostConfig.NetworkMode != "host" {
		nsenterArgs = append(nsenterArgs, "--net")
	}
	nsenterArgs = append(nsenterArgs, "--")

	cmdArgs := remaining[1:]
	if *workdir != "" {
		cmdArgs = []string{"sh", "-c", fmt.Sprintf("cd %s && exec %s", *workdir, strings.Join(cmdArgs, " "))}
	}
	nsenterArgs = append(nsenterArgs, cmdArgs...)

	cmd := exec.Command("nsenter", nsenterArgs...)
	if *interactive || *tty {
		cmd.Stdin = os.Stdin
	}
	if !*detach {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if len(envs) > 0 {
		cmd.Env = append(os.Environ(), envs...)
	}
	_ = *user

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "cryruss: exec: %v\n", err)
		os.Exit(1)
	}
}

func cmdLogs(args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "Follow log output")
	tail := fs.String("tail", "all", "Number of lines from end")
	timestamps := fs.Bool("t", false, "Show timestamps")
	fs.Parse(args)
	_ = *timestamps

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "container ID required")
		os.Exit(1)
	}

	mgr := container.NewManager()
	c, err := mgr.Get(fs.Args()[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Open(c.LogPath)
	if err != nil {
		return
	}
	defer f.Close()

	if *tail != "all" {
		n, _ := strconv.Atoi(*tail)
		if n > 0 {
			

			data, _ := io.ReadAll(f)
			lines := strings.Split(string(data), "\n")
			if len(lines) > n {
				lines = lines[len(lines)-n:]
			}
			fmt.Print(strings.Join(lines, "\n"))
			return
		}
	}

	io.Copy(os.Stdout, f)

	if *follow {
		for {
			time.Sleep(200 * time.Millisecond)
			io.Copy(os.Stdout, f)
			if !c.State.Running {
				break
			}
		}
	}
}

func cmdInspect(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	format := fs.String("f", "", "Format using Go template")
	typeFlag := fs.String("type", "", "Return JSON for specified type (container|image|volume|network)")
	fs.Parse(args)
	_ = *format

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "name or ID required")
		os.Exit(1)
	}

	var results []any
	for _, id := range fs.Args() {
		var obj any
		var err error

		switch *typeFlag {
		case "image":
			obj, err = image.NewManager().Get(id)
		case "volume":
			obj, err = volume.NewManager().Get(id)
		case "network":
			obj, err = network.NewManager().Get(id)
		default:
			

			c, cerr := container.NewManager().Get(id)
			if cerr == nil {
				obj = c
			} else {
				img, ierr := image.NewManager().Get(id)
				if ierr == nil {
					obj = img
				} else {
					err = fmt.Errorf("no such object: %s", id)
				}
			}
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			os.Exit(1)
		}
		results = append(results, obj)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	enc.Encode(results)
}

func cmdKill(args []string) {
	fs := flag.NewFlagSet("kill", flag.ExitOnError)
	sig := fs.String("s", "KILL", "Signal to send")
	fs.Parse(args)

	mgr := container.NewManager()
	for _, id := range fs.Args() {
		c, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		signal := syscall.SIGKILL
		switch strings.ToUpper(*sig) {
		case "TERM", "SIGTERM":
			signal = syscall.SIGTERM
		case "INT", "SIGINT":
			signal = syscall.SIGINT
		case "HUP", "SIGHUP":
			signal = syscall.SIGHUP
		}
		rt.Stop(c, signal, 0)
		fmt.Println(c.ID)
	}
}

func cmdPause(args []string) {
	mgr := container.NewManager()
	for _, id := range args {
		c, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		if c.State.Pid > 0 {
			proc, _ := os.FindProcess(c.State.Pid)
			if proc != nil {
				proc.Signal(syscall.SIGSTOP)
			}
		}
		mgr.UpdateState(c.ID, func(c *container.Container) {
			c.State.Paused = true
			c.State.Status = container.StatusPaused
		})
		fmt.Println(c.ID)
	}
}

func cmdUnpause(args []string) {
	mgr := container.NewManager()
	for _, id := range args {
		c, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		if c.State.Pid > 0 {
			proc, _ := os.FindProcess(c.State.Pid)
			if proc != nil {
				proc.Signal(syscall.SIGCONT)
			}
		}
		mgr.UpdateState(c.ID, func(c *container.Container) {
			c.State.Paused = false
			c.State.Status = container.StatusRunning
		})
		fmt.Println(c.ID)
	}
}

func cmdRename(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cryruss rename CONTAINER NEW_NAME")
		os.Exit(1)
	}
	mgr := container.NewManager()
	c, err := mgr.Get(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	newName := args[1]
	if !strings.HasPrefix(newName, "/") {
		newName = "/" + newName
	}
	c.Names = []string{newName}
	mgr.Save(c)
}

func cmdStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	noStream := fs.Bool("no-stream", false, "Disable streaming stats and only pull the first result")
	_ = noStream
	fs.Parse(args)

	mgr := container.NewManager()
	containers, _ := mgr.List(false)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tNAME\tCPU %\tMEM USAGE / LIMIT\tMEM %\tPIDS")
	for _, c := range containers {
		stats := rt.ReadCgroupStats(c.ID)
		memUsage := stats["memory_usage"]
		memLimit := c.HostConfig.Memory
		memPct := 0.0
		if memLimit > 0 && memUsage > 0 {
			memPct = float64(memUsage) / float64(memLimit) * 100
		}
		memUsageStr := image.FormatSize(memUsage)
		memLimitStr := "unlimited"
		if memLimit > 0 {
			memLimitStr = image.FormatSize(memLimit)
		}
		pids := stats["pids_current"]
		fmt.Fprintf(w, "%s\t%s\t%.2f%%\t%s / %s\t%.1f%%\t%d\n",
			c.ID[:12],
			strings.Join(c.Names, ","),
			0.0, 

			memUsageStr, memLimitStr,
			memPct,
			pids,
		)
	}
	w.Flush()
}

func cmdTop(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "container ID required")
		os.Exit(1)
	}
	mgr := container.NewManager()
	c, err := mgr.Get(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	if c.State.Pid <= 0 {
		fmt.Println("UID   PID   PPID   C   STIME   TTY   TIME   CMD")
		return
	}
	

	cmd := exec.Command("ps", "-o", "uid,pid,ppid,c,stime,tty,time,cmd", "--ppid", fmt.Sprint(c.State.Pid))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func cmdWait(args []string) {
	mgr := container.NewManager()
	for _, id := range args {
		c, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		for c.State.Running {
			time.Sleep(500 * time.Millisecond)
			c, _ = mgr.Get(id)
		}
		fmt.Println(c.State.ExitCode)
	}
}

func cmdPort(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "container ID required")
		os.Exit(1)
	}
	mgr := container.NewManager()
	c, err := mgr.Get(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	for port, bindings := range c.HostConfig.PortBindings {
		for _, b := range bindings {
			fmt.Printf("%s -> %s:%s\n", port, b.HostIP, b.HostPort)
		}
	}
}

func cmdDiff(args []string) {
	fmt.Println("(diff not implemented in non-overlay mode)")
}

func cmdCp(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cryruss cp CONTAINER:PATH DEST | cryruss cp SRC CONTAINER:PATH")
		os.Exit(1)
	}
	

	src := args[0]
	dst := args[1]

	mgr := container.NewManager()

	

	if strings.Contains(src, ":") {
		

		parts := strings.SplitN(src, ":", 2)
		c, err := mgr.Get(parts[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			os.Exit(1)
		}
		srcPath := filepath.Join(c.RootfsPath, parts[1])
		cmd := exec.Command("cp", "-a", srcPath, dst)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	} else {
		

		parts := strings.SplitN(dst, ":", 2)
		c, err := mgr.Get(parts[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			os.Exit(1)
		}
		dstPath := filepath.Join(c.RootfsPath, parts[1])
		cmd := exec.Command("cp", "-a", src, dstPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}

func cmdImages(args []string) {
	fs := flag.NewFlagSet("images", flag.ExitOnError)
	all := fs.Bool("a", false, "Show all images")
	quiet := fs.Bool("q", false, "Only show image IDs")
	noTrunc := fs.Bool("no-trunc", false, "Don't truncate output")
	format := fs.String("format", "", "Format output")
	fs.Parse(args)
	_ = *all

	mgr := image.NewManager()
	images, err := mgr.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}

	if *quiet {
		for _, img := range images {
			id := image.ShortID(img.ID)
			if *noTrunc {
				id = img.ID
			}
			fmt.Println(id)
		}
		return
	}
	if *format != "" {
		for _, img := range images {
			out := *format
			out = strings.ReplaceAll(out, "{{.ID}}", image.ShortID(img.ID))
			out = strings.ReplaceAll(out, "{{.Repository}}", repoName(img))
			out = strings.ReplaceAll(out, "{{.Tag}}", tagName(img))
			fmt.Println(out)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE")
	for _, img := range images {
		id := image.ShortID(img.ID)
		if *noTrunc {
			id = img.ID
		}
		created := time.Since(time.Unix(img.Created, 0))
		repo := repoName(img)
		tag := tagName(img)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s ago\t%s\n",
			repo, tag, id, formatDur(created), image.FormatSize(img.Size))
	}
	w.Flush()
}

func cmdPull(args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	platform := fs.String("platform", "", "Set platform (e.g. linux/amd64)")
	fs.Parse(args)
	_ = *platform

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "image name required")
		os.Exit(1)
	}

	ref := fs.Args()[0]
	mgr := image.NewManager()

	fmt.Printf("Pulling from \x1b[1m%s\x1b[0m\n", ref)

	

	type layerState struct {
		id      string
		current int64
		total   int64
		done    bool
	}

	var (
		mu     sync.Mutex
		order  []string
		layers = map[string]*layerState{}
	)

	barWidth := 30

	renderBar := func(cur, tot int64) string {
		if tot <= 0 {
			return "[" + strings.Repeat("-", barWidth) + "]"
		}
		pct := float64(cur) / float64(tot)
		if pct > 1 {
			pct = 1
		}
		filled := int(pct * float64(barWidth))
		return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled) + "]"
	}

	fmtBytes := func(b int64) string {
		const unit = 1024
		if b < unit {
			return fmt.Sprintf("%dB", b)
		}
		div, exp := int64(unit), 0
		for n := b / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
	}

	redraw := func() {
		mu.Lock()
		defer mu.Unlock()
		n := len(order)
		if n == 0 {
			return
		}
		fmt.Printf("\x1b[%dA", n)
		for _, id := range order {
			ls := layers[id]
			if ls.done {
				fmt.Printf("\x1b[2K\x1b[32m%s\x1b[0m: Pull complete\n", id)
			} else if ls.total > 0 {
				bar := renderBar(ls.current, ls.total)
				fmt.Printf("\x1b[2K\x1b[36m%s\x1b[0m: Downloading %s %s/%s\n",
					id, bar, fmtBytes(ls.current), fmtBytes(ls.total))
			} else {
				fmt.Printf("\x1b[2K\x1b[33m%s\x1b[0m: Waiting...\n", id)
			}
		}
	}

	progressFn := func(p image.PullProgress) {
		mu.Lock()
		_, exists := layers[p.Layer]
		if !exists {
			layers[p.Layer] = &layerState{id: p.Layer}
			order = append(order, p.Layer)
			mu.Unlock()
			fmt.Printf("\x1b[33m%s\x1b[0m: Pulling fs layer\n", p.Layer)
			mu.Lock()
		}
		ls := layers[p.Layer]
		ls.current = p.Current
		ls.total = p.Total
		ls.done = p.Done
		mu.Unlock()
		redraw()
	}

	img, err := mgr.Pull(ref, progressFn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\ncryruss: pull: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("\x1b[1mDigest:\x1b[0m %s\n", img.Digest)
	fmt.Printf("\x1b[1mStatus:\x1b[0m Downloaded newer image for %s\n", ref)
}

func cmdRmi(args []string) {
	fs := flag.NewFlagSet("rmi", flag.ExitOnError)
	force := fs.Bool("f", false, "Force removal")
	fs.Parse(args)

	mgr := image.NewManager()
	for _, id := range fs.Args() {
		if err := mgr.Delete(id, *force); err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		fmt.Printf("Untagged: %s\n", id)
	}
}

func cmdTag(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cryruss tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]")
		os.Exit(1)
	}
	mgr := image.NewManager()
	if err := mgr.Tag(args[0], args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
}

func cmdImageInspect(args []string) {
	cmdInspect(append([]string{"--type=image"}, args...))
}

func cmdImagePrune(args []string) {
	fmt.Println("No images pruned")
}

func cmdImageHistory(args []string) {
	if len(args) == 0 {
		return
	}
	mgr := image.NewManager()
	img, err := mgr.Get(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "IMAGE\tCREATED\tCREATED BY\tSIZE\tCOMMENT")
	fmt.Fprintf(w, "%s\t%s\t\t%s\t\n",
		image.ShortID(img.ID),
		time.Since(time.Unix(img.Created, 0)).String()+" ago",
		image.FormatSize(img.Size))
	w.Flush()
}

func cmdNetwork(args []string) {
	if len(args) == 0 {
		fmt.Print(`Usage:  cryruss network COMMAND

Manage networks

Commands:
  connect     Connect a container to a network
  create      Create a network
  disconnect  Disconnect a container from a network
  inspect     Display detailed information on one or more networks
  ls          List networks
  prune       Remove all unused networks
  rm          Remove one or more networks
`)
		return
	}
	switch args[0] {
	case "ls", "list":
		cmdNetworkList(args[1:])
	case "create":
		cmdNetworkCreate(args[1:])
	case "rm", "remove":
		cmdNetworkRm(args[1:])
	case "inspect":
		cmdNetworkInspect(args[1:])
	case "connect":
		cmdNetworkConnect(args[1:])
	case "disconnect":
		cmdNetworkDisconnect(args[1:])
	case "prune":
		cmdNetworkPrune(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "cryruss: 'network %s' is not a cryruss network command\n", args[0])
		os.Exit(1)
	}
}

func cmdNetworkList(args []string) {
	fs := flag.NewFlagSet("network ls", flag.ExitOnError)
	quiet := fs.Bool("q", false, "Only display network IDs")
	noTrunc := fs.Bool("no-trunc", false, "Do not truncate the output")
	var filters multiFlag
	fs.Var(&filters, "f", "Provide filter values (e.g. 'driver=bridge')")
	fs.Var(&filters, "filter", "Provide filter values")
	format := fs.String("format", "", "Format output using a custom template")
	fs.Parse(args)
	_ = *format

	

	filterMap := parseKeyValueList(filters)

	mgr := network.NewManager()
	networks, _ := mgr.List()

	

	var filtered []*network.Network
	for _, n := range networks {
		if len(filterMap) > 0 {
			match := true
			for k, v := range filterMap {
				switch k {
				case "driver":
					if n.Driver != v {
						match = false
					}
				case "name":
					if !strings.Contains(n.Name, v) {
						match = false
					}
				case "id":
					if !strings.HasPrefix(n.ID, v) {
						match = false
					}
				case "type":
					if v == "custom" && (n.Name == "bridge" || n.Name == "host" || n.Name == "none") {
						match = false
					}
					if v == "builtin" && n.Name != "bridge" && n.Name != "host" && n.Name != "none" {
						match = false
					}
				}
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, n)
	}

	if *quiet {
		for _, n := range filtered {
			id := n.ID
			if !*noTrunc && len(id) > 12 {
				id = id[:12]
			}
			fmt.Println(id)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NETWORK ID\tNAME\tDRIVER\tSCOPE")
	for _, n := range filtered {
		id := n.ID
		if !*noTrunc && len(id) > 12 {
			id = id[:12]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", id, n.Name, n.Driver, n.Scope)
	}
	w.Flush()
}

func cmdNetworkCreate(args []string) {
	fs := flag.NewFlagSet("network create", flag.ExitOnError)
	driver := fs.String("d", "bridge", "Driver to manage the Network")
	fs.String("driver", "bridge", "Driver to manage the Network")
	internal := fs.Bool("internal", false, "Restrict external access to the network")
	attachable := fs.Bool("attachable", false, "Enable manual container attachment")
	ipv6 := fs.Bool("ipv6", false, "Enable IPv6 networking")
	subnet := fs.String("subnet", "", "Subnet in CIDR format")
	gateway := fs.String("gateway", "", "IPv4 or IPv6 Gateway for the master subnet")
	ipRange := fs.String("ip-range", "", "Allocate container ip from a sub-range")
	var labels multiFlag
	fs.Var(&labels, "label", "Set metadata on a network")
	var opts multiFlag
	fs.Var(&opts, "o", "Set driver specific options")
	fs.Var(&opts, "opt", "Set driver specific options")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Error: \"network create\" requires exactly 1 argument")
		os.Exit(1)
	}

	var ipam *network.IPAM
	if *subnet != "" || *gateway != "" || *ipRange != "" {
		cfg := network.IPAMConfig{Subnet: *subnet, Gateway: *gateway, IPRange: *ipRange}
		ipam = &network.IPAM{Driver: "default", Config: []network.IPAMConfig{cfg}, Options: map[string]string{}}
	}

	mgr := network.NewManager()
	n, err := mgr.Create(&network.CreateRequest{
		Name:           fs.Args()[0],
		Driver:         *driver,
		Internal:       *internal,
		Attachable:     *attachable,
		EnableIPv6:     *ipv6,
		IPAM:           ipam,
		Labels:         parseKeyValueList(labels),
		Options:        parseKeyValueList(opts),
		CheckDuplicate: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Println(n.ID)
}

func cmdNetworkRm(args []string) {
	fs := flag.NewFlagSet("network rm", flag.ExitOnError)
	force := fs.Bool("f", false, "Force the removal of one or more networks")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Error: \"network rm\" requires at least 1 argument")
		os.Exit(1)
	}

	mgr := network.NewManager()
	exitCode := 0
	for _, id := range fs.Args() {
		if err := mgr.DeleteForce(id, *force); err != nil {
			fmt.Fprintf(os.Stderr, "Error response from daemon: %v\n", err)
			exitCode = 1
			continue
		}
		fmt.Println(id)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func cmdNetworkInspect(args []string) {
	fs := flag.NewFlagSet("network inspect", flag.ExitOnError)
	format := fs.String("f", "", "Format the output using the given Go template")
	fs.String("format", "", "Format the output using the given Go template")
	verbose := fs.Bool("v", false, "Verbose output for diagnostics")
	fs.Parse(args)
	_ = *format
	_ = *verbose

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Error: \"network inspect\" requires at least 1 argument")
		os.Exit(1)
	}

	mgr := network.NewManager()
	var results []any
	exitCode := 0
	for _, id := range fs.Args() {
		n, err := mgr.Get(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			exitCode = 1
			continue
		}
		results = append(results, n)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	enc.Encode(results)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func cmdNetworkConnect(args []string) {
	fs := flag.NewFlagSet("network connect", flag.ExitOnError)
	var aliases multiFlag
	fs.Var(&aliases, "alias", "Add network-scoped alias for the container")
	ip4 := fs.String("ip", "", "IPv4 address")
	ip6 := fs.String("ip6", "", "IPv6 address")
	var links multiFlag
	fs.Var(&links, "link", "Add link to another container")
	fs.Parse(args)
	_ = *ip6

	if len(fs.Args()) < 2 {
		fmt.Fprintln(os.Stderr, "Usage:  cryruss network connect [OPTIONS] NETWORK CONTAINER")
		os.Exit(1)
	}
	netID := fs.Args()[0]
	ctrID := fs.Args()[1]

	cMgr := container.NewManager()
	c, err := cMgr.Get(ctrID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: No such container: %s\n", ctrID)
		os.Exit(1)
	}
	_ = links

	nMgr := network.NewManager()
	req := &network.ConnectRequest{
		Container: c.ID,
		Aliases:   aliases,
		IPv4:      *ip4,
	}
	if err := nMgr.Connect(netID, c.ID, strings.TrimPrefix(c.Names[0], "/"), req); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func cmdNetworkDisconnect(args []string) {
	fs := flag.NewFlagSet("network disconnect", flag.ExitOnError)
	force := fs.Bool("f", false, "Force the container to disconnect from a network")
	fs.Parse(args)

	if len(fs.Args()) < 2 {
		fmt.Fprintln(os.Stderr, "Usage:  cryruss network disconnect [OPTIONS] NETWORK CONTAINER")
		os.Exit(1)
	}
	netID := fs.Args()[0]
	ctrID := fs.Args()[1]

	cMgr := container.NewManager()
	c, err := cMgr.Get(ctrID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: No such container: %s\n", ctrID)
		os.Exit(1)
	}

	nMgr := network.NewManager()
	if err := nMgr.Disconnect(netID, c.ID, *force); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func cmdNetworkPrune(args []string) {
	fs := flag.NewFlagSet("network prune", flag.ExitOnError)
	force := fs.Bool("f", false, "Do not prompt for confirmation")
	fs.Parse(args)

	if !*force {
		fmt.Print("WARNING! This will remove all custom networks not used by at least one container.\nAre you sure you want to continue? [y/N] ")
		var resp string
		fmt.Scanln(&resp)
		if resp != "y" && resp != "Y" {
			return
		}
	}

	mgr := network.NewManager()
	pruned, err := mgr.Prune()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Deleted Networks:")
	for _, name := range pruned {
		fmt.Println(name)
	}
	if len(pruned) == 0 {
		fmt.Println("(none)")
	}
}

func cmdVolume(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cryruss volume COMMAND")
		fmt.Println("Commands: ls, create, rm, inspect, prune")
		return
	}
	switch args[0] {
	case "ls", "list":
		cmdVolumeList(args[1:])
	case "create":
		cmdVolumeCreate(args[1:])
	case "rm", "remove":
		cmdVolumeRm(args[1:])
	case "inspect":
		cmdVolumeInspect(args[1:])
	case "prune":
		cmdVolumePrune(args[1:])
	}
}

func cmdVolumeList(args []string) {
	fs := flag.NewFlagSet("volume ls", flag.ExitOnError)
	quiet := fs.Bool("q", false, "Only display volume names")
	fs.Parse(args)

	mgr := volume.NewManager()
	volumes, _ := mgr.List()

	if *quiet {
		for _, v := range volumes {
			fmt.Println(v.Name)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "DRIVER\tVOLUME NAME")
	for _, v := range volumes {
		fmt.Fprintf(w, "%s\t%s\n", v.Driver, v.Name)
	}
	w.Flush()
}

func cmdVolumeCreate(args []string) {
	fs := flag.NewFlagSet("volume create", flag.ExitOnError)
	driver := fs.String("d", "local", "Volume driver")
	name := fs.String("name", "", "Volume name")
	fs.Parse(args)

	n := *name
	if n == "" && len(fs.Args()) > 0 {
		n = fs.Args()[0]
	}

	mgr := volume.NewManager()
	v, err := mgr.Create(&volume.CreateRequest{Name: n, Driver: *driver})
	if err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(v.Name)
}

func cmdVolumeRm(args []string) {
	fs := flag.NewFlagSet("volume rm", flag.ExitOnError)
	force := fs.Bool("f", false, "Force removal")
	fs.Parse(args)

	mgr := volume.NewManager()
	for _, name := range fs.Args() {
		if err := mgr.Delete(name, *force); err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		fmt.Println(name)
	}
}

func cmdVolumeInspect(args []string) {
	mgr := volume.NewManager()
	var results []any
	for _, name := range args {
		v, err := mgr.Get(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cryruss: %v\n", err)
			continue
		}
		results = append(results, v)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "    ")
	enc.Encode(results)
}

func cmdVolumePrune(args []string) {
	mgr := volume.NewManager()
	removed, freed, _ := mgr.Prune()
	for _, name := range removed {
		fmt.Println("Deleted:", name)
	}
	fmt.Printf("Total reclaimed space: %s\n", image.FormatSize(freed))
}

func cmdSystem(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: cryruss system COMMAND")
		fmt.Println("Commands: info, prune, df")
		return
	}
	switch args[0] {
	case "info":
		cmdInfo(args[1:])
	case "prune":
		cmdSystemPrune(args[1:])
	case "df":
		cmdSystemDf(args[1:])
	}
}

func cmdInfo(args []string) {
	cMgr := container.NewManager()
	iMgr := image.NewManager()
	containers, _ := cMgr.List(true)
	images, _ := iMgr.List()
	running := 0
	for _, c := range containers {
		if c.State.Running {
			running++
		}
	}
	fmt.Printf("Client:\n")
	fmt.Printf(" Version: %s\n", cryruss.Version)
	fmt.Printf(" Go version: %s\n", runtime.Version())
	fmt.Printf(" OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("\nServer:\n")
	fmt.Printf(" Containers: %d\n", len(containers))
	fmt.Printf("  Running: %d\n", running)
	fmt.Printf("  Stopped: %d\n", len(containers)-running)
	fmt.Printf(" Images: %d\n", len(images))
	fmt.Printf(" Storage Driver: copy\n")
	fmt.Printf(" Logging Driver: json-file\n")
	fmt.Printf(" Cgroup Driver: none\n")
	fmt.Printf(" Kernel: nonroot user namespaces\n")
	fmt.Printf(" Operating System: %s\n", runtime.GOOS)
	fmt.Printf(" Architecture: %s\n", runtime.GOARCH)
	fmt.Printf(" CPUs: %d\n", runtime.NumCPU())
}

func cmdSystemPrune(args []string) {
	fs := flag.NewFlagSet("system prune", flag.ExitOnError)
	volumes := fs.Bool("volumes", false, "Prune volumes")
	fs.Parse(args)

	cMgr := container.NewManager()
	containers, _ := cMgr.List(true)
	pruned := 0
	for _, c := range containers {
		if !c.State.Running {
			cMgr.Delete(c.ID)
			pruned++
		}
	}
	fmt.Printf("Deleted containers: %d\n", pruned)
	if *volumes {
		vMgr := volume.NewManager()
		removed, freed, _ := vMgr.Prune()
		fmt.Printf("Deleted volumes: %d\n", len(removed))
		fmt.Printf("Total reclaimed space: %s\n", image.FormatSize(freed))
	}
}

func cmdSystemDf(args []string) {
	cMgr := container.NewManager()
	iMgr := image.NewManager()
	vMgr := volume.NewManager()

	containers, _ := cMgr.List(true)
	images, _ := iMgr.List()
	volumes, _ := vMgr.List()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TYPE\tTOTAL\tACTIVE\tSIZE\tRECLAIMABLE")
	fmt.Fprintf(w, "Images\t%d\t-\t-\t-\n", len(images))
	fmt.Fprintf(w, "Containers\t%d\t-\t-\t-\n", len(containers))
	fmt.Fprintf(w, "Local Volumes\t%d\t-\t-\t-\n", len(volumes))
	w.Flush()
}

func cmdVersion(args []string) {
	fmt.Printf("cryruss version %s\n", cryruss.Version)
	fmt.Printf("API version %s\n", cryruss.APIVersion)
	fmt.Printf("Go version %s\n", runtime.Version())
	fmt.Printf("OS/Arch %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	socketPath := fs.String("socket", "", "Unix socket path (default: ~/.local/share/cryruss/cryruss.sock)")
	fs.Parse(args)

	if *socketPath != "" {
		config.Global.SocketPath = *socketPath
	}

	

	os.Remove(config.Global.SocketPath)

	srv := api.NewServer()
	fmt.Printf("cryruss API listening on unix://%s\n", config.Global.SocketPath)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		os.Remove(config.Global.SocketPath)
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "cryruss: api: %v\n", err)
		os.Exit(1)
	}
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func parseMemory(s string) int64 {
	if s == "" {
		return 0
	}
	s = strings.ToLower(s)
	mul := int64(1)
	if strings.HasSuffix(s, "g") {
		mul = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "m") {
		mul = 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "k") {
		mul = 1024
		s = s[:len(s)-1]
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n * mul
}

func parseMounts(volumes []string) []string {
	return volumes
}

func parsePortBindings(ports []string) container.PortMap {
	pm := container.PortMap{}
	for _, p := range ports {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) == 2 {
			containerPort := parts[1]
			if !strings.Contains(containerPort, "/") {
				containerPort += "/tcp"
			}
			pm[containerPort] = append(pm[containerPort], container.PortBinding{
				HostIP:   "0.0.0.0",
				HostPort: parts[0],
			})
		}
	}
	return pm
}

func parseLabels(label string) map[string]string {
	m := map[string]string{}
	if label == "" {
		return m
	}
	for _, part := range strings.Split(label, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	return m
}

func mergeEnv(base, override []string) []string {
	result := make([]string, len(base))
	copy(result, base)
	for _, e := range override {
		key := strings.SplitN(e, "=", 2)[0]
		found := false
		for i, b := range result {
			if strings.HasPrefix(b, key+"=") {
				result[i] = e
				found = true
				break
			}
		}
		if !found {
			result = append(result, e)
		}
	}
	return result
}

func formatDur(d time.Duration) string {
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

func formatPorts(c *container.Container) string {
	var parts []string
	for port, bindings := range c.HostConfig.PortBindings {
		for _, b := range bindings {
			parts = append(parts, fmt.Sprintf("%s:%s->%s", b.HostIP, b.HostPort, port))
		}
	}
	return strings.Join(parts, ", ")
}

func repoName(img *image.Image) string {
	if len(img.RepoTags) == 0 {
		return "<none>"
	}
	ref := ParseRef(img.RepoTags[0])
	return ref[0]
}

func tagName(img *image.Image) string {
	if len(img.RepoTags) == 0 {
		return "<none>"
	}
	ref := ParseRef(img.RepoTags[0])
	if len(ref) < 2 {
		return "latest"
	}
	return ref[1]
}

func ParseRef(ref string) []string {
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		return []string{ref[:idx], ref[idx+1:]}
	}
	return []string{ref, "latest"}
}

func hasRootfs(path string) bool {
	for _, d := range []string{"bin", "usr", "etc", "lib"} {
		if _, err := os.Stat(filepath.Join(path, d)); err == nil {
			return true
		}
	}
	return false
}

func parseKeyValueList(pairs []string) map[string]string {
	m := map[string]string{}
	for _, p := range pairs {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		} else if len(parts) == 1 && parts[0] != "" {
			m[parts[0]] = ""
		}
	}
	return m
}

func parseUlimits(specs []string) []container.Ulimit {
	var out []container.Ulimit
	for _, s := range specs {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[0]
		limits := strings.SplitN(parts[1], ":", 2)
		var soft, hard int64
		fmt.Sscanf(limits[0], "%d", &soft)
		hard = soft
		if len(limits) == 2 {
			fmt.Sscanf(limits[1], "%d", &hard)
		}
		out = append(out, container.Ulimit{Name: name, Soft: soft, Hard: hard})
	}
	return out
}

func readEnvFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		env = append(env, line)
	}
	return env, nil
}

func cmdStack(args []string) {
	if len(args) == 0 {
		fmt.Print(`Usage:  cryruss stack COMMAND

Manage Stacks

Commands:
  deploy      Deploy a new stack or update an existing stack
  ls          List stacks
  ps          List the tasks in the stack
  rm          Remove one or more stacks
  services    List the services in the stack
`)
		return
	}
	switch args[0] {
	case "deploy", "up":
		cmdStackDeploy(args[1:])
	case "ls", "list":
		cmdStackList(args[1:])
	case "rm", "remove", "down":
		cmdStackRm(args[1:])
	case "ps":
		cmdStackPs(args[1:])
	case "services":
		cmdStackServices(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "cryruss: 'stack %s' is not a cryruss stack command\n", args[0])
		os.Exit(1)
	}
}

const stackLabel = "com.docker.stack.namespace"
const stackServiceLabel = "com.docker.stack.service"

func cmdStackDeploy(args []string) {
	fs := flag.NewFlagSet("stack deploy", flag.ExitOnError)
	composeFile := fs.String("c", "", "Path to a Compose file")
	fs.String("compose-file", "", "Path to a Compose file")
	prune := fs.Bool("prune", false, "Prune services that are no longer referenced")
	resolveImage := fs.String("resolve-image", "always", "Query the registry to resolve image digest (always|changed|never)")
	fs.Parse(args)
	_ = *prune
	_ = *resolveImage

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Error: \"stack deploy\" requires exactly 1 argument")
		os.Exit(1)
	}
	stackName := fs.Args()[0]

	cf := *composeFile
	if cf == "" {
		

		for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
			if _, err := os.Stat(name); err == nil {
				cf = name
				break
			}
		}
		if cf == "" {
			fmt.Fprintln(os.Stderr, "Error: please specify a Compose file with -c")
			os.Exit(1)
		}
	}

	compose, err := parseComposeFile(cf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deploying stack '%s' from '%s'\n", stackName, cf)

	

	nMgr := network.NewManager()
	for netName := range compose.Networks {
		fullName := stackName + "_" + netName
		if _, err := nMgr.Get(fullName); err != nil {
			_, err := nMgr.Create(&network.CreateRequest{
				Name:   fullName,
				Driver: "bridge",
				Labels: map[string]string{stackLabel: stackName},
			})
			if err == nil {
				fmt.Printf("Creating network %s\n", fullName)
			}
		}
	}

	

	vMgr := volume.NewManager()
	for volName := range compose.Volumes {
		fullName := stackName + "_" + volName
		if _, err := vMgr.Get(fullName); err != nil {
			vMgr.Create(&volume.CreateRequest{
				Name:   fullName,
				Driver: "local",
				Labels: map[string]string{stackLabel: stackName},
			})
			fmt.Printf("Creating volume %s\n", fullName)
		}
	}

	

	cMgr := container.NewManager()
	imgMgr := image.NewManager()

	for svcName, svc := range compose.Services {
		fullName := stackName + "_" + svcName
		fmt.Printf("Creating service %s\n", fullName)

		

		img, err := imgMgr.Get(svc.Image)
		if err != nil && svc.Image != "" {
			fmt.Printf("Pulling %s (%s)\n", svcName, svc.Image)
			img, err = imgMgr.Pull(svc.Image, func(p image.PullProgress) {})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not pull %s: %v\n", svc.Image, err)
				continue
			}
		}
		imgID := ""
		if img != nil {
			imgID = img.ID
		}

		replicas := svc.Deploy.Replicas
		if replicas == 0 {
			replicas = 1
		}

		for i := 0; i < replicas; i++ {
			suffix := fmt.Sprintf("%d", i+1)
			ctrName := fullName + "." + suffix

			labels := map[string]string{
				stackLabel:        stackName,
				stackServiceLabel: svcName,
			}
			for k, v := range svc.Labels {
				labels[k] = v
			}

			netMode := "bridge"
			if len(svc.Networks) > 0 {
				for n := range svc.Networks {
					netMode = stackName + "_" + n
					break
				}
			}

			req := &container.CreateRequest{
				Image:      svc.Image,
				ImageID:    imgID,
				Cmd:        svc.Command,
				Env:        svc.Environment,
				Labels:     labels,
				WorkingDir: svc.WorkingDir,
				Hostname:   svc.Hostname,
				HostConfig: container.HostConfig{
					NetworkMode:   netMode,
					Binds:         svc.Volumes,
					RestartPolicy: container.RestartPolicy{Name: svc.Restart},
				},
			}

			c, err := cMgr.Create(req, ctrName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: create %s: %v\n", ctrName, err)
				continue
			}

			if img != nil {
				if err := imgMgr.PrepareRootfs(img, c.RootfsPath); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: unpack %s: %v\n", ctrName, err)
				}
			}
		}
	}
	fmt.Printf("\nStack '%s' deployed successfully\n", stackName)
}

func cmdStackList(args []string) {
	cMgr := container.NewManager()
	ctrs, _ := cMgr.ListAll()

	stacks := map[string]int{}
	for _, c := range ctrs {
		if ns, ok := c.Labels[stackLabel]; ok {
			stacks[ns]++
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSERVICES")
	

	services := map[string]map[string]bool{}
	for _, c := range ctrs {
		ns, ok := c.Labels[stackLabel]
		if !ok {
			continue
		}
		svc := c.Labels[stackServiceLabel]
		if services[ns] == nil {
			services[ns] = map[string]bool{}
		}
		services[ns][svc] = true
	}
	for ns, svcs := range services {
		fmt.Fprintf(w, "%s\t%d\n", ns, len(svcs))
	}
	w.Flush()
}

func cmdStackRm(args []string) {
	fs := flag.NewFlagSet("stack rm", flag.ExitOnError)
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Error: \"stack rm\" requires at least 1 argument")
		os.Exit(1)
	}

	cMgr := container.NewManager()
	_ = image.NewManager()

	for _, stackName := range fs.Args() {
		fmt.Printf("Removing stack '%s'\n", stackName)
		ctrs, _ := cMgr.ListAll()
		for _, c := range ctrs {
			if c.Labels[stackLabel] != stackName {
				continue
			}
			name := strings.TrimPrefix(c.Names[0], "/")
			fmt.Printf("Removing service %s\n", name)
			if c.State.Running {
				rt.Stop(c, syscall.SIGTERM, 5)
			}
			os.RemoveAll(c.RootfsPath)
			cMgr.Delete(c.ID)
		}

		

		nMgr := network.NewManager()
		nets, _ := nMgr.List()
		for _, n := range nets {
			if n.Labels[stackLabel] == stackName {
				nMgr.DeleteForce(n.ID, true)
				fmt.Printf("Removing network %s\n", n.Name)
			}
		}

		

		vMgr := volume.NewManager()
		vols, _ := vMgr.List()
		for _, v := range vols {
			if v.Labels[stackLabel] == stackName {
				vMgr.Delete(v.Name, true)
				fmt.Printf("Removing volume %s\n", v.Name)
			}
		}
	}
}

func cmdStackPs(args []string) {
	fs := flag.NewFlagSet("stack ps", flag.ExitOnError)
	quiet := fs.Bool("q", false, "Only display IDs")
	noTrunc := fs.Bool("no-trunc", false, "Do not truncate output")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Error: \"stack ps\" requires exactly 1 argument")
		os.Exit(1)
	}
	stackName := fs.Args()[0]

	cMgr := container.NewManager()
	ctrs, _ := cMgr.ListAll()

	if *quiet {
		for _, c := range ctrs {
			if c.Labels[stackLabel] != stackName {
				continue
			}
			id := c.ID
			if !*noTrunc && len(id) > 12 {
				id = id[:12]
			}
			fmt.Println(id)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tIMAGE\tNODE\tDESIRED STATE\tCURRENT STATE")
	for _, c := range ctrs {
		if c.Labels[stackLabel] != stackName {
			continue
		}
		id := c.ID
		if !*noTrunc && len(id) > 12 {
			id = id[:12]
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		state := string(c.State.Status)
		desired := "Running"
		fmt.Fprintf(w, "%s\t%s\t%s\t-\t%s\t%s\n", id, name, c.Image, desired, state)
	}
	w.Flush()
}

func cmdStackServices(args []string) {
	fs := flag.NewFlagSet("stack services", flag.ExitOnError)
	quiet := fs.Bool("q", false, "Only display IDs")
	fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Error: \"stack services\" requires exactly 1 argument")
		os.Exit(1)
	}
	stackName := fs.Args()[0]

	cMgr := container.NewManager()
	ctrs, _ := cMgr.ListAll()

	services := map[string][]*container.Container{}
	for _, c := range ctrs {
		if c.Labels[stackLabel] != stackName {
			continue
		}
		svc := c.Labels[stackServiceLabel]
		services[svc] = append(services[svc], c)
	}

	if *quiet {
		for svc := range services {
			fmt.Println(svc)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tMODE\tREPLICAS\tIMAGE\tPORTS")
	for svc, cs := range services {
		id := cs[0].ID
		if len(id) > 12 {
			id = id[:12]
		}
		img := cs[0].Image
		fullName := stackName + "_" + svc
		fmt.Fprintf(w, "%s\t%s\treplicated\t%d/%d\t%s\t\n",
			id, fullName, len(cs), len(cs), img)
	}
	w.Flush()
}

type ComposeFile struct {
	Version  string
	Services map[string]*ComposeService
	Networks map[string]*ComposeNetwork
	Volumes  map[string]*ComposeVolume
}

type ComposeService struct {
	Image       string
	Build       string
	Command     []string
	Entrypoint  []string
	Environment []string
	Labels      map[string]string
	Volumes     []string
	Ports       []string
	Networks    map[string]struct{}
	WorkingDir  string
	Hostname    string
	Restart     string
	DependsOn   []string
	Deploy      ComposeDeploy
	EnvFile     []string
	User        string
	Privileged  bool
	Tty         bool
	Stdin       bool
	CapAdd      []string
	CapDrop     []string
}

type ComposeDeploy struct {
	Replicas int
	Mode     string
}

type ComposeNetwork struct {
	Driver     string
	External   bool
	Name       string
	Attachable bool
}

type ComposeVolume struct {
	Driver   string
	External bool
	Name     string
}

func parseComposeFile(path string) (*ComposeFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read compose file: %w", err)
	}

	cf := &ComposeFile{
		Services: map[string]*ComposeService{},
		Networks: map[string]*ComposeNetwork{},
		Volumes:  map[string]*ComposeVolume{},
	}

	lines := strings.Split(string(data), "\n")

	

	const (
		stateRoot     = "root"
		stateServices = "services"
		stateSvc      = "service"
		stateSvcSub   = "service_sub" 

		stateNetworks = "networks"
		stateNet      = "network"
		stateVolumes  = "volumes"
		stateVol      = "volume"
		stateDeploy   = "deploy"
	)

	state := stateRoot
	var curSvcName string
	var curNetName string
	var curVolName string
	var curSubKey string 

	indent := func(line string) int {
		n := 0
		for _, c := range line {
			if c == ' ' {
				n++
			} else {
				break
			}
		}
		return n
	}

	trimLine := func(line string) string {
		

		if idx := strings.Index(line, " #"); idx > 0 {
			line = strings.TrimRight(line[:idx], " \t")
		}
		return strings.TrimSpace(line)
	}

	parseScalar := func(s string) string {
		s = strings.TrimSpace(s)
		if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
			s = s[1 : len(s)-1]
		}
		return s
	}

	for _, rawLine := range lines {
		if strings.TrimSpace(rawLine) == "" || strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			continue
		}
		lvl := indent(rawLine)
		line := trimLine(rawLine)
		if line == "" {
			continue
		}

		

		keyOnly := strings.HasSuffix(line, ":") && !strings.Contains(line, ": ")

		kv := strings.SplitN(line, ": ", 2)
		key := strings.TrimSuffix(kv[0], ":")
		val := ""
		if len(kv) == 2 {
			val = parseScalar(kv[1])
		}

		

		if lvl == 0 {
			switch key {
			case "version":
				cf.Version = val
				state = stateRoot
			case "services":
				state = stateServices
			case "networks":
				state = stateNetworks
			case "volumes":
				state = stateVolumes
			default:
				state = stateRoot
			}
			continue
		}

		

		if state == stateServices || state == stateSvc || state == stateSvcSub || state == stateDeploy {
			if lvl == 2 {
				

				curSvcName = key
				if _, ok := cf.Services[curSvcName]; !ok {
					cf.Services[curSvcName] = &ComposeService{
						Networks: map[string]struct{}{},
						Labels:   map[string]string{},
					}
				}
				state = stateSvc
				curSubKey = ""
				continue
			}
			if lvl == 4 && state != stateDeploy {
				svc := cf.Services[curSvcName]
				if svc == nil {
					continue
				}
				curSubKey = key
				state = stateSvc
				if !keyOnly && val != "" {
					

					switch key {
					case "image":
						svc.Image = val
					case "build":
						svc.Build = val
					case "working_dir", "workingdir":
						svc.WorkingDir = val
					case "hostname":
						svc.Hostname = val
					case "restart":
						svc.Restart = val
					case "user":
						svc.User = val
					case "privileged":
						svc.Privileged = val == "true"
					case "tty":
						svc.Tty = val == "true"
					case "stdin_open":
						svc.Stdin = val == "true"
					case "command":
						svc.Command = strings.Fields(val)
					case "entrypoint":
						svc.Entrypoint = strings.Fields(val)
					}
				} else if keyOnly {
					switch key {
					case "deploy":
						state = stateDeploy
					case "environment", "volumes", "ports", "labels", "networks",
						"depends_on", "cap_add", "cap_drop", "env_file", "command", "entrypoint":
						state = stateSvcSub
					}
				}
				continue
			}
			if lvl == 4 && state == stateDeploy {
				

				state = stateSvc
				svc := cf.Services[curSvcName]
				curSubKey = key
				if !keyOnly && val != "" {
					switch key {
					case "replicas":
						fmt.Sscanf(val, "%d", &svc.Deploy.Replicas)
					case "mode":
						svc.Deploy.Mode = val
					}
				}
				continue
			}
			if (lvl == 6 || lvl == 8) && (state == stateSvcSub || state == stateSvc) {
				svc := cf.Services[curSvcName]
				if svc == nil {
					continue
				}
				item := strings.TrimPrefix(line, "- ")
				switch curSubKey {
				case "environment":
					if val != "" {
						svc.Environment = append(svc.Environment, key+"="+val)
					} else {
						svc.Environment = append(svc.Environment, item)
					}
				case "volumes":
					svc.Volumes = append(svc.Volumes, item)
				case "ports":
					svc.Ports = append(svc.Ports, item)
				case "networks":
					netName := strings.TrimPrefix(item, "- ")
					if netName != "" {
						svc.Networks[netName] = struct{}{}
					}
				case "depends_on":
					svc.DependsOn = append(svc.DependsOn, item)
				case "cap_add":
					svc.CapAdd = append(svc.CapAdd, item)
				case "cap_drop":
					svc.CapDrop = append(svc.CapDrop, item)
				case "env_file":
					svc.EnvFile = append(svc.EnvFile, item)
				case "command":
					svc.Command = append(svc.Command, item)
				case "entrypoint":
					svc.Entrypoint = append(svc.Entrypoint, item)
				case "labels":
					if val != "" {
						svc.Labels[key] = val
					} else {
						parts := strings.SplitN(item, "=", 2)
						if len(parts) == 2 {
							svc.Labels[parts[0]] = parts[1]
						}
					}
				}
				continue
			}
		}

		

		if state == stateNetworks || state == stateNet {
			if lvl == 2 {
				curNetName = key
				if _, ok := cf.Networks[curNetName]; !ok {
					cf.Networks[curNetName] = &ComposeNetwork{}
				}
				state = stateNet
				continue
			}
			if lvl == 4 {
				n := cf.Networks[curNetName]
				if n == nil {
					continue
				}
				switch key {
				case "driver":
					n.Driver = val
				case "external":
					n.External = val == "true"
				case "name":
					n.Name = val
				case "attachable":
					n.Attachable = val == "true"
				}
				continue
			}
		}

		

		if state == stateVolumes || state == stateVol {
			if lvl == 2 {
				curVolName = key
				if _, ok := cf.Volumes[curVolName]; !ok {
					cf.Volumes[curVolName] = &ComposeVolume{}
				}
				state = stateVol
				continue
			}
			if lvl == 4 {
				v := cf.Volumes[curVolName]
				if v == nil {
					continue
				}
				switch key {
				case "driver":
					v.Driver = val
				case "external":
					v.External = val == "true"
				case "name":
					v.Name = val
				}
				continue
			}
		}
	}

	return cf, nil
}

func expandCombinedFlags(args []string) []string {
	

	combinable := map[byte]bool{
		'a': true, 

		'd': true, 

		'f': true, 

		'i': true, 

		'q': true, 

		't': true, 

		'v': true, 

	}

	out := make([]string, 0, len(args))
	for _, arg := range args {
		

		if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
			chars := arg[1:] 

			allComb := true
			for i := 0; i < len(chars); i++ {
				if !combinable[chars[i]] {
					allComb = false
					break
				}
			}
			if allComb {
				for i := 0; i < len(chars); i++ {
					out = append(out, "-"+string(chars[i]))
				}
				continue
			}
		}
		out = append(out, arg)
	}
	return out
}

func parseBlkioThrottleDevices(specs []string) []container.ThrottleDevice {
	var out []container.ThrottleDevice
	for _, s := range specs {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			continue
		}
		rate := parseRateBytes(parts[1])
		out = append(out, container.ThrottleDevice{Path: parts[0], Rate: rate})
	}
	return out
}

func parseBlkioWeightDevices(specs []string) []container.BlkioWeightDevice {
	var out []container.BlkioWeightDevice
	for _, s := range specs {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			continue
		}
		var w uint16
		fmt.Sscanf(parts[1], "%d", &w)
		out = append(out, container.BlkioWeightDevice{Path: parts[0], Weight: w})
	}
	return out
}

func parseRateBytes(s string) int64 {
	s = strings.ToLower(strings.TrimSpace(s))
	mul := int64(1)
	if strings.HasSuffix(s, "gb") || strings.HasSuffix(s, "g") {
		mul = 1024 * 1024 * 1024
		s = strings.TrimRight(s, "gb")
	} else if strings.HasSuffix(s, "mb") || strings.HasSuffix(s, "m") {
		mul = 1024 * 1024
		s = strings.TrimRight(s, "mb")
	} else if strings.HasSuffix(s, "kb") || strings.HasSuffix(s, "k") {
		mul = 1024
		s = strings.TrimRight(s, "kb")
	} else {
		s = strings.TrimRight(s, "b")
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n * mul
}
