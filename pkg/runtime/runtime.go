//go:build linux


package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cryruss/cryruss/pkg/config"
	"github.com/cryruss/cryruss/pkg/container"
)

const InitArg = "__cryruss_init__"

// IsRestrictedKernel returns true when running inside proot, a user-mode
// Linux environment, or any other context where namespace syscalls (unshare /
// clone with CLONE_NEW*) are not permitted.  It probes by attempting an
// unshare that should always be cheap to reverse.
func IsRestrictedKernel() bool {
	// Try CLONE_NEWUTS — the least-invasive namespace to probe with.
	// In a real kernel with user-namespaces enabled this succeeds silently
	// (we are in a new UTS namespace for just this goroutine-OS-thread, and
	// Go's runtime will re-exec anyway).  In proot/chroot it returns EINVAL.
	err := syscall.Unshare(syscall.CLONE_NEWUTS)
	return err != nil
}

type InitConfig struct {
	Rootfs      string       `json:"Rootfs"`
	Args        []string     `json:"Args"`
	Env         []string     `json:"Env"`
	Hostname    string       `json:"Hostname"`
	Workdir     string       `json:"Workdir"`
	User        string       `json:"User"`
	NetworkMode string       `json:"NetworkMode"`
	Tty         bool         `json:"Tty"`
	Mounts      []MountPoint `json:"Mounts"`
	ReadOnly    bool         `json:"ReadOnly"`
	AddHosts    []string     `json:"AddHosts"`
	DNS         []string     `json:"DNS"`
	DNSSearch   []string     `json:"DNSSearch"`
	NSFlags     int          `json:"NSFlags"` 

}

type MountPoint struct {
	Source   string
	Target   string
	Type     string
	ReadOnly bool
}

func ContainerInit() {
	configFile := os.NewFile(3, "config")
	syncFile := os.NewFile(4, "sync")
	readyFile := os.NewFile(5, "ready")

	

	

	

	

	defaultFlags := syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
		syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC

	if err := syscall.Unshare(defaultFlags); err != nil {
		// Namespace creation failed (e.g. running inside proot).
		// Try a minimal unshare — just mount namespace — so we can still
		// pivot_root / chroot safely.  If even that fails, continue without
		// any namespace isolation (chroot-only mode).
		if err2 := syscall.Unshare(syscall.CLONE_NEWNS); err2 != nil {
			// Completely restricted — proceed with chroot-only isolation.
			fmt.Fprintf(os.Stderr, "init: warning: namespace isolation unavailable (%v), using chroot-only mode\n", err)
		}
	}

	

	syncFile.Write([]byte("ready\n"))
	syncFile.Close()

	

	buf := make([]byte, 4)
	readyFile.Read(buf)
	readyFile.Close()

	

	var cfg InitConfig
	if err := json.NewDecoder(configFile).Decode(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "init: read config: %v\n", err)
		os.Exit(127)
	}
	configFile.Close()

	

	

	

	grandchild, _, errno := syscall.RawSyscall(syscall.SYS_CLONE, uintptr(syscall.SIGCHLD), 0, 0)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "init: fork: %v\n", errno)
		os.Exit(127)
	}
	if grandchild != 0 {
		

		var ws syscall.WaitStatus
		syscall.Wait4(int(grandchild), &ws, 0, nil)
		if ws.Exited() {
			os.Exit(ws.ExitStatus())
		}
		os.Exit(1)
	}
	

	

	

	if cfg.NSFlags != 0 {
		extra := cfg.NSFlags &^ (syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC)
		if extra != 0 {
			_ = syscall.Unshare(extra) 

		}
	}

	

	if cfg.Hostname != "" {
		syscall.Sethostname([]byte(cfg.Hostname)) 

	}

	

	if err := setupMounts(cfg.Rootfs, cfg.Mounts, cfg.ReadOnly); err != nil {
		fmt.Fprintf(os.Stderr, "init: mounts: %v\n", err)
		os.Exit(127)
	}

	

	writeEtcFiles(cfg)

	

	if err := pivotRoot(cfg.Rootfs); err != nil {
		if err2 := syscall.Chroot(cfg.Rootfs); err2 != nil {
			fmt.Fprintf(os.Stderr, "init: chroot: %v\n", err2)
			os.Exit(127)
		}
		syscall.Chdir("/")
	}

	workdir := cfg.Workdir
	if workdir == "" {
		workdir = "/"
	}
	if err := os.Chdir(workdir); err != nil {
		os.Chdir("/")
	}

	env := cfg.Env
	if len(env) == 0 {
		env = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=/root",
			"TERM=xterm",
		}
	}

	args := cfg.Args
	if len(args) == 0 {
		args = []string{"/bin/sh"}
	}

	binary, err := lookupBinary(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: exec: %v\n", err)
		os.Exit(127)
	}

	if err := syscall.Exec(binary, args, env); err != nil {
		fmt.Fprintf(os.Stderr, "init: exec %s: %v\n", binary, err)
		os.Exit(127)
	}
}

func lookupBinary(name string) (string, error) {
	if filepath.IsAbs(name) {
		return name, nil
	}
	for _, p := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin"} {
		full := filepath.Join(p, name)
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}
	return name, nil
}

func setupMounts(rootfs string, extra []MountPoint, readOnly bool) error {
	

	syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")

	

	syscall.Mount(rootfs, rootfs, "bind", syscall.MS_BIND|syscall.MS_REC, "")

	

	for _, d := range []string{"proc", "sys", "dev", "dev/pts", "dev/shm", "tmp", "etc", "run"} {
		os.MkdirAll(filepath.Join(rootfs, d), 0755)
	}

	

	if err := syscall.Mount("proc", filepath.Join(rootfs, "proc"), "proc",
		syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
		

		

		_ = err
	}

	

	syscall.Mount("sysfs", filepath.Join(rootfs, "sys"), "sysfs",
		syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC|syscall.MS_RDONLY, "")

	

	syscall.Mount("tmpfs", filepath.Join(rootfs, "dev"), "tmpfs",
		syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755,size=65536k")

	

	os.MkdirAll(filepath.Join(rootfs, "dev/pts"), 0755)
	syscall.Mount("devpts", filepath.Join(rootfs, "dev/pts"), "devpts",
		syscall.MS_NOSUID|syscall.MS_NOEXEC,
		"newinstance,ptmxmode=0666,mode=0620")

	

	os.MkdirAll(filepath.Join(rootfs, "dev/shm"), 0755)
	syscall.Mount("shm", filepath.Join(rootfs, "dev/shm"), "tmpfs",
		syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777,size=65536k")

	

	syscall.Mount("tmpfs", filepath.Join(rootfs, "tmp"), "tmpfs",
		syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777")

	createDevices(filepath.Join(rootfs, "dev"))

	

	

	_ = readOnly

	

	for _, mp := range extra {
		target := filepath.Join(rootfs, mp.Target)
		if info, err := os.Stat(mp.Source); err == nil && info.IsDir() {
			os.MkdirAll(target, 0755)
		} else {
			os.MkdirAll(filepath.Dir(target), 0755)
			if _, err := os.Stat(target); os.IsNotExist(err) {
				f, _ := os.Create(target)
				if f != nil {
					f.Close()
				}
			}
		}
		flags := uintptr(syscall.MS_BIND | syscall.MS_REC)
		syscall.Mount(mp.Source, target, "bind", flags, "")
		if mp.ReadOnly {
			syscall.Mount(mp.Source, target, "bind", flags|syscall.MS_RDONLY|syscall.MS_REMOUNT, "")
		}
	}

	return nil
}

func createDevices(devDir string) {
	

	devlinks := map[string]string{
		"stdin":  "/proc/self/fd/0",
		"stdout": "/proc/self/fd/1",
		"stderr": "/proc/self/fd/2",
		"fd":     "/proc/self/fd",
		"ptmx":   "pts/ptmx",
	}
	for name, target := range devlinks {
		os.Symlink(target, filepath.Join(devDir, name))
	}

	

	hostDevs := []string{"null", "zero", "full", "random", "urandom", "tty"}
	for _, name := range hostDevs {
		src := filepath.Join("/dev", name)
		dst := filepath.Join(devDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		

		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0666)
		if err == nil {
			f.Close()
		}
		syscall.Mount(src, dst, "bind", syscall.MS_BIND, "")
	}
}

func pivotRoot(rootfs string) error {
	putold := filepath.Join(rootfs, ".pivot_root")
	if err := os.MkdirAll(putold, 0700); err != nil {
		return err
	}
	if err := syscall.PivotRoot(rootfs, putold); err != nil {
		os.Remove(putold)
		return err
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	putold = "/.pivot_root"
	if err := syscall.Unmount(putold, syscall.MNT_DETACH); err != nil {
		return err
	}
	return os.Remove(putold)
}

func writeEtcFiles(cfg InitConfig) {
	etcDir := filepath.Join(cfg.Rootfs, "etc")
	os.MkdirAll(etcDir, 0755)

	

	hosts := "127.0.0.1\tlocalhost\n::1\tlocalhost ip6-localhost ip6-loopback\n"
	if cfg.Hostname != "" {
		hosts += fmt.Sprintf("127.0.1.1\t%s\n", cfg.Hostname)
	}
	for _, h := range cfg.AddHosts {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			hosts += fmt.Sprintf("%s\t%s\n", parts[1], parts[0])
		}
	}
	os.WriteFile(filepath.Join(etcDir, "hosts"), []byte(hosts), 0644)

	

	if len(cfg.DNS) > 0 || len(cfg.DNSSearch) > 0 {
		var resolv strings.Builder
		for _, dns := range cfg.DNS {
			fmt.Fprintf(&resolv, "nameserver %s\n", dns)
		}
		for _, s := range cfg.DNSSearch {
			fmt.Fprintf(&resolv, "search %s\n", s)
		}
		os.WriteFile(filepath.Join(etcDir, "resolv.conf"), []byte(resolv.String()), 0644)
	}
}

type RunOptions struct {
	Detach      bool
	Interactive bool
	TTY         bool
}

func uidMapSize() int {
	uid := os.Getuid()
	if n := readSubIDCount("/etc/subuid", uid); n > 0 {
		return n
	}
	return 1
}

func readSubIDCount(file string, uid int) int {
	data, err := os.ReadFile(file)
	if err != nil {
		return 0
	}
	uidStr := strconv.Itoa(uid)
	

	username := ""
	if u, err := userLookupUID(uid); err == nil {
		username = u
	}
	best := 0
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(strings.TrimSpace(line), ":")
		if len(parts) != 3 {
			continue
		}
		if parts[0] == uidStr || (username != "" && parts[0] == username) {
			if n, err := strconv.Atoi(parts[2]); err == nil && n > best {
				best = n
			}
		}
	}
	return best
}

func userLookupUID(uid int) (string, error) {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return "", err
	}
	uidStr := strconv.Itoa(uid)
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) >= 4 && parts[2] == uidStr {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("uid %d not found", uid)
}

func hasNewuidmap() bool {
	_, err := exec.LookPath("newuidmap")
	return err == nil
}

func buildUIDMappings(uid, gid int) ([]syscall.SysProcIDMap, []syscall.SysProcIDMap) {
	subUIDStart, subUIDCount := readSubIDRange("/etc/subuid", uid)
	subGIDStart, subGIDCount := readSubIDRange("/etc/subgid", gid)

	var uidMaps, gidMaps []syscall.SysProcIDMap

	if subUIDCount > 0 {
		

		

		uidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
			{ContainerID: 1, HostID: subUIDStart, Size: subUIDCount},
		}
	} else {
		

		

		

		

		

		uidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
			

			

			{ContainerID: 1, HostID: uid + 1, Size: 65535},
		}
	}

	if subGIDCount > 0 {
		gidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: gid, Size: 1},
			{ContainerID: 1, HostID: subGIDStart, Size: subGIDCount},
		}
	} else {
		gidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: gid, Size: 1},
			{ContainerID: 1, HostID: gid + 1, Size: 65535},
		}
	}

	return uidMaps, gidMaps
}

func readSubIDRange(file string, id int) (int, int) {
	data, err := os.ReadFile(file)
	if err != nil {
		return 0, 0
	}
	idStr := strconv.Itoa(id)
	username := ""
	if u, err := userLookupUID(id); err == nil {
		username = u
	}
	bestStart, bestCount := 0, 0
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(strings.TrimSpace(line), ":")
		if len(parts) != 3 {
			continue
		}
		if parts[0] != idStr && (username == "" || parts[0] != username) {
			continue
		}
		start, err1 := strconv.Atoi(parts[1])
		count, err2 := strconv.Atoi(parts[2])
		if err1 == nil && err2 == nil && count > bestCount {
			bestStart = start
			bestCount = count
		}
	}
	return bestStart, bestCount
}

// Start launches a container.  In restricted kernel environments (proot,
// unprivileged chroot, …) where namespace syscalls are blocked it delegates
// transparently to StartWithProot so callers need no special flags.
func Start(c *container.Container, opts RunOptions) (*os.Process, error) {
	if IsRestrictedKernel() {
		return StartWithProot(c, opts)
	}
	return startNative(c, opts)
}

// startNative is the real namespace-based launcher.  It must never call
// StartWithProot — that would create infinite mutual recursion.
func startNative(c *container.Container, opts RunOptions) (*os.Process, error) {
	args := resolveArgs(c)
	initCfg := &InitConfig{
		Rootfs:      c.RootfsPath,
		Args:        args,
		Env:         buildEnv(c),
		Hostname:    c.Config.Hostname,
		Workdir:     c.Config.WorkingDir,
		User:        c.Config.User,
		NetworkMode: c.HostConfig.NetworkMode,
		Tty:         c.Config.Tty,
		Mounts:      buildMounts(c),
		ReadOnly:    c.HostConfig.ReadonlyRootfs,
		AddHosts:    c.HostConfig.ExtraHosts,
		DNS:         c.HostConfig.DNS,
		DNSSearch:   c.HostConfig.DNSSearch,
	}
	if initCfg.Workdir == "" {
		initCfg.Workdir = "/"
	}

	uid := os.Getuid()
	gid := os.Getgid()
	uidMaps, gidMaps := buildUIDMappings(uid, gid)

	

	configR, configW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	

	syncR, syncW, err := os.Pipe()
	if err != nil {
		configR.Close()
		configW.Close()
		return nil, err
	}
	

	readyR, readyW, err := os.Pipe()
	if err != nil {
		configR.Close()
		configW.Close()
		syncR.Close()
		syncW.Close()
		return nil, err
	}

	

	nsFlags := syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
		syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC
	if c.HostConfig.NetworkMode != "host" {
		nsFlags |= syscall.CLONE_NEWNET
	}
	initCfg.NSFlags = nsFlags

	

	cmd := exec.Command("/proc/self/exe", InitArg)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		

		Pdeathsig: syscall.SIGKILL,
	}
	cmd.ExtraFiles = []*os.File{configR, syncW, readyR}

	if opts.Interactive || opts.TTY {
		cmd.Stdin = os.Stdin
	}
	logFile, _ := os.OpenFile(c.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if opts.Detach {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		configR.Close()
		configW.Close()
		syncR.Close()
		syncW.Close()
		readyR.Close()
		readyW.Close()
		return nil, fmt.Errorf("start container process: %w", err)
	}

	

	configR.Close()
	syncW.Close()
	readyR.Close()

	

	buf := make([]byte, 6)
	if _, err := syncR.Read(buf); err != nil {
		cmd.Process.Kill()
		syncR.Close()
		configW.Close()
		readyW.Close()
		return nil, fmt.Errorf("sync with child: %w", err)
	}
	syncR.Close()

	

	

	

	pid := cmd.Process.Pid
	if err := writeUIDMaps(pid, uid, gid, uidMaps, gidMaps); err != nil {
		

		_ = err
	}

	

	readyW.Write([]byte("go\n"))
	readyW.Close()

	

	if err := json.NewEncoder(configW).Encode(initCfg); err != nil {
		configW.Close()
		cmd.Process.Kill()
		return nil, err
	}
	configW.Close()

	

	ApplyCgroups(pid, c.HostConfig, c.ID)

	if c.HostConfig.NetworkMode != "host" && c.HostConfig.NetworkMode != "none" {
		setupSlirp4netns(pid)
	}

	pidFile := filepath.Join(config.Global.ContainersDir, c.ID, "pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)

	return cmd.Process, nil
}

func writeUIDMaps(pid, uid, gid int, uidMaps, gidMaps []syscall.SysProcIDMap) error {
	procDir := fmt.Sprintf("/proc/%d", pid)

	

	var uidLines []string
	for _, m := range uidMaps {
		uidLines = append(uidLines, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
	}
	

	var gidLines []string
	for _, m := range gidMaps {
		gidLines = append(gidLines, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
	}

	

	if len(uidMaps) > 1 && hasNewuidmap() && uid != 0 {
		return applyNewuidmap(pid, uidMaps, gidMaps)
	}

	

	if err := os.WriteFile(procDir+"/uid_map", []byte(strings.Join(uidLines, "\n")), 0); err != nil {
		return fmt.Errorf("uid_map: %w", err)
	}
	

	_ = os.WriteFile(procDir+"/setgroups", []byte("allow"), 0)
	if err := os.WriteFile(procDir+"/gid_map", []byte(strings.Join(gidLines, "\n")), 0); err != nil {
		return fmt.Errorf("gid_map: %w", err)
	}
	return nil
}

func StartWithProot(c *container.Container, opts RunOptions) (*os.Process, error) {
	prootBin, err := exec.LookPath("proot")
	if err != nil {
		// proot binary not found.  Do NOT fall back to Start — that would
		// create infinite mutual recursion when namespaces are unavailable.
		return nil, fmt.Errorf("proot not found in PATH and kernel namespaces are unavailable; " +
			"install proot (e.g. 'apt install proot') to run containers in this environment")
	}

	args := resolveArgs(c)
	prootArgs := []string{
		"--rootfs=" + c.RootfsPath,
		"--bind=/dev",
		"--bind=/proc",
		"--bind=/sys",
		"--change-id=0:0", 

	}

	

	for _, mp := range buildMounts(c) {
		if mp.ReadOnly {
			prootArgs = append(prootArgs, fmt.Sprintf("--bind=%s:%s", mp.Source, mp.Target))
		} else {
			prootArgs = append(prootArgs, fmt.Sprintf("--bind=%s:%s", mp.Source, mp.Target))
		}
	}

	if c.Config.WorkingDir != "" {
		prootArgs = append(prootArgs, "--cwd="+c.Config.WorkingDir)
	}

	prootArgs = append(prootArgs, "--") 

	prootArgs = append(prootArgs, args...)

	cmd := exec.Command(prootBin, prootArgs...)
	cmd.Env = buildEnv(c)

	if opts.Interactive || opts.TTY {
		cmd.Stdin = os.Stdin
	}

	logFile, _ := os.OpenFile(c.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if opts.Detach {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("proot start: %w", err)
	}

	

	ApplyCgroups(cmd.Process.Pid, c.HostConfig, c.ID)

	pidFile := filepath.Join(config.Global.ContainersDir, c.ID, "pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

	return cmd.Process, nil
}

func applyNewuidmap(pid int, uidMaps, gidMaps []syscall.SysProcIDMap) error {
	

	args := []string{strconv.Itoa(pid)}
	for _, m := range uidMaps {
		args = append(args,
			strconv.Itoa(m.ContainerID),
			strconv.Itoa(m.HostID),
			strconv.Itoa(m.Size),
		)
	}
	if out, err := exec.Command("newuidmap", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("newuidmap: %v: %s", err, out)
	}

	

	setgroupsPath := fmt.Sprintf("/proc/%d/setgroups", pid)
	_ = os.WriteFile(setgroupsPath, []byte("allow"), 0)

	gargs := []string{strconv.Itoa(pid)}
	for _, m := range gidMaps {
		gargs = append(gargs,
			strconv.Itoa(m.ContainerID),
			strconv.Itoa(m.HostID),
			strconv.Itoa(m.Size),
		)
	}
	if out, err := exec.Command("newgidmap", gargs...).CombinedOutput(); err != nil {
		return fmt.Errorf("newgidmap: %v: %s", err, out)
	}
	return nil
}

func writeUIDMapDirect(pid int, uidMaps, gidMaps []syscall.SysProcIDMap) error {
	if uidMaps != nil {
		var lines []string
		for _, m := range uidMaps {
			lines = append(lines, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
		}
		if err := os.WriteFile(fmt.Sprintf("/proc/%d/uid_map", pid),
			[]byte(strings.Join(lines, "\n")), 0); err != nil {
			return err
		}
	}
	if gidMaps != nil {
		var lines []string
		for _, m := range gidMaps {
			lines = append(lines, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
		}
		if err := os.WriteFile(fmt.Sprintf("/proc/%d/gid_map", pid),
			[]byte(strings.Join(lines, "\n")), 0); err != nil {
			return err
		}
	}
	return nil
}

func setupSlirp4netns(pid int) {
	_, err := exec.LookPath("slirp4netns")
	if err != nil {
		return 

	}
	

	slirp := exec.Command("slirp4netns",
		"--configure",
		"--mtu=65520",
		"--disable-host-loopback",
		strconv.Itoa(pid),
		"tap0",
	)
	slirp.Stdout = nil
	slirp.Stderr = nil
	slirp.Start() 

}

func resolveArgs(c *container.Container) []string {
	ep := c.Config.Entrypoint
	cm := c.Config.Cmd
	if len(ep) == 0 && len(cm) == 0 {
		return []string{"/bin/sh"}
	}
	return append(ep, cm...)
}

func buildEnv(c *container.Container) []string {
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
		"HOSTNAME=" + c.Config.Hostname,
		"TERM=xterm",
	}
	env = append(env, c.Config.Env...)
	return env
}

func buildMounts(c *container.Container) []MountPoint {
	var mounts []MountPoint
	for _, bind := range c.HostConfig.Binds {
		parts := strings.SplitN(bind, ":", 3)
		if len(parts) < 2 {
			continue
		}
		mp := MountPoint{Source: parts[0], Target: parts[1]}
		if len(parts) == 3 && strings.Contains(parts[2], "ro") {
			mp.ReadOnly = true
		}
		mounts = append(mounts, mp)
	}
	return mounts
}

func Stop(c *container.Container, signal syscall.Signal, timeout int) error {
	if c.State.Pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(c.State.Pid)
	if err != nil {
		return nil
	}
	proc.Signal(signal)

	done := make(chan struct{})
	go func() { proc.Wait(); close(done) }()

	select {
	case <-done:
		return nil
	case <-time.After(time.Duration(timeout) * time.Second):
		proc.Kill()
	}
	return nil
}

func GetPID(containerID string) int {
	b, err := os.ReadFile(filepath.Join(config.Global.ContainersDir, containerID, "pid"))
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(string(b), "%d", &pid)
	return pid
}

func IsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
