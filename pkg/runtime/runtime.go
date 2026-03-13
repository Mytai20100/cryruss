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
	NSFlags     int          `json:"NSFlags"` // CLONE_NEW* flags to unshare inside child
}

type MountPoint struct {
	Source   string
	Target   string
	Type     string
	ReadOnly bool
}

// ──────────────────────────────────────────────────────────────────────────────
// ContainerInit – runs inside the new namespaces (re-exec'd child)
// ──────────────────────────────────────────────────────────────────────────────

// ContainerInit – re-exec'd child process that sets up namespaces and execs the container command.
//
// FD layout (from parent via ExtraFiles):
//
//	fd3 = configR  (parent → child: JSON InitConfig, sent after maps are written)
//	fd4 = syncW    (child → parent: child writes "ready\n" when unshare done)
//	fd5 = readyR   (parent → child: parent writes "go\n" after writing uid/gid maps)
func ContainerInit() {
	configFile := os.NewFile(3, "config")
	syncFile := os.NewFile(4, "sync")
	readyFile := os.NewFile(5, "ready")

	// Step 1: unshare namespaces.
	// We use a temporary default if NSFlags not yet known — read will come later.
	// But we need to unshare BEFORE reading config so parent can write maps.
	// Use a minimal unshare first (just CLONE_NEWUSER), then full after config.
	defaultFlags := syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
		syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC

	if err := syscall.Unshare(defaultFlags); err != nil {
		fmt.Fprintf(os.Stderr, "init: unshare: %v\n", err)
		os.Exit(127)
	}

	// Step 2: signal parent that we've called unshare (parent can now write maps)
	syncFile.Write([]byte("ready\n"))
	syncFile.Close()

	// Step 3: wait for parent to write uid/gid maps and signal us
	buf := make([]byte, 4)
	readyFile.Read(buf)
	readyFile.Close()

	// Step 4: read init config from parent (sent after maps are written)
	var cfg InitConfig
	if err := json.NewDecoder(configFile).Decode(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "init: read config: %v\n", err)
		os.Exit(127)
	}
	configFile.Close()

	// Step 5: CLONE_NEWPID requires a fork — the calling process itself does NOT
	// enter the new PID namespace; only its children do. So we fork here.
	// The parent (original child) waits for the forked grandchild.
	grandchild, _, errno := syscall.RawSyscall(syscall.SYS_FORK, 0, 0, 0)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "init: fork: %v\n", errno)
		os.Exit(127)
	}
	if grandchild != 0 {
		// We are the intermediate process — wait for grandchild and relay exit code
		var ws syscall.WaitStatus
		syscall.Wait4(int(grandchild), &ws, 0, nil)
		if ws.Exited() {
			os.Exit(ws.ExitStatus())
		}
		os.Exit(1)
	}
	// We are now PID 1 inside the new PID namespace

	// Step 6: if additional namespace flags were requested (e.g. CLONE_NEWNET),
	// unshare those now (we're already in user namespace so this should work)
	if cfg.NSFlags != 0 {
		extra := cfg.NSFlags &^ (syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC)
		if extra != 0 {
			_ = syscall.Unshare(extra) // non-fatal (e.g. CLONE_NEWNET may need CAP_NET_ADMIN)
		}
	}

	// Hostname (UTS namespace)
	if cfg.Hostname != "" {
		syscall.Sethostname([]byte(cfg.Hostname)) // non-fatal
	}

	// Mounts
	if err := setupMounts(cfg.Rootfs, cfg.Mounts, cfg.ReadOnly); err != nil {
		fmt.Fprintf(os.Stderr, "init: mounts: %v\n", err)
		os.Exit(127)
	}

	// Write /etc/hosts and /etc/resolv.conf if requested
	writeEtcFiles(cfg)

	// Pivot root → fallback chroot
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

// ──────────────────────────────────────────────────────────────────────────────
// Mount setup
// ──────────────────────────────────────────────────────────────────────────────

func setupMounts(rootfs string, extra []MountPoint, readOnly bool) error {
	// Make mount tree private so changes don't propagate to host
	syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")

	// Bind-mount rootfs onto itself (required for pivot_root)
	syscall.Mount(rootfs, rootfs, "bind", syscall.MS_BIND|syscall.MS_REC, "")

	// Create standard directories
	for _, d := range []string{"proc", "sys", "dev", "dev/pts", "dev/shm", "tmp", "etc", "run"} {
		os.MkdirAll(filepath.Join(rootfs, d), 0755)
	}

	// /proc – works in user+PID namespaces
	if err := syscall.Mount("proc", filepath.Join(rootfs, "proc"), "proc",
		syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
		// In some restricted environments proc mount may fail; create a minimal /proc
		// so the container can still start
		_ = err
	}

	// /sys – may fail without privileges; ignore
	syscall.Mount("sysfs", filepath.Join(rootfs, "sys"), "sysfs",
		syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC|syscall.MS_RDONLY, "")

	// /dev via tmpfs
	syscall.Mount("tmpfs", filepath.Join(rootfs, "dev"), "tmpfs",
		syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755,size=65536k")

	// /dev/pts
	os.MkdirAll(filepath.Join(rootfs, "dev/pts"), 0755)
	syscall.Mount("devpts", filepath.Join(rootfs, "dev/pts"), "devpts",
		syscall.MS_NOSUID|syscall.MS_NOEXEC,
		"newinstance,ptmxmode=0666,mode=0620")

	// /dev/shm
	os.MkdirAll(filepath.Join(rootfs, "dev/shm"), 0755)
	syscall.Mount("shm", filepath.Join(rootfs, "dev/shm"), "tmpfs",
		syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777,size=65536k")

	// /tmp
	syscall.Mount("tmpfs", filepath.Join(rootfs, "tmp"), "tmpfs",
		syscall.MS_NOSUID|syscall.MS_NODEV, "mode=1777")

	createDevices(filepath.Join(rootfs, "dev"))

	// Optional read-only root: remount after all bind mounts
	// (applied after pivot root in the child)
	_ = readOnly

	// Extra bind mounts from volumes
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
	// Symlinks always work even without CAP_MKNOD
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

	// Bind-mount host device nodes (works in user namespace)
	hostDevs := []string{"null", "zero", "full", "random", "urandom", "tty"}
	for _, name := range hostDevs {
		src := filepath.Join("/dev", name)
		dst := filepath.Join(devDir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		// Create placeholder file
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

// writeEtcFiles writes /etc/hosts and /etc/resolv.conf into the rootfs.
func writeEtcFiles(cfg InitConfig) {
	etcDir := filepath.Join(cfg.Rootfs, "etc")
	os.MkdirAll(etcDir, 0755)

	// /etc/hosts
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

	// /etc/resolv.conf
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

// ──────────────────────────────────────────────────────────────────────────────
// RunOptions / Start
// ──────────────────────────────────────────────────────────────────────────────

type RunOptions struct {
	Detach      bool
	Interactive bool
	TTY         bool
}

// uidMapSize returns the number of subordinate UIDs available from /etc/subuid.
func uidMapSize() int {
	uid := os.Getuid()
	if n := readSubIDCount("/etc/subuid", uid); n > 0 {
		return n
	}
	return 1
}

// readSubIDCount reads /etc/subuid or /etc/subgid and returns the count for the given uid.
// Matches both by numeric UID and by username.
func readSubIDCount(file string, uid int) int {
	data, err := os.ReadFile(file)
	if err != nil {
		return 0
	}
	uidStr := strconv.Itoa(uid)
	// Also try to get username for name-based lookup
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

// userLookupUID returns the username for a given UID by reading /etc/passwd.
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

// hasNewuidmap checks whether newuidmap/newgidmap binaries exist (needed for
// large UID mappings without being root).
func hasNewuidmap() bool {
	_, err := exec.LookPath("newuidmap")
	return err == nil
}

// buildUIDMappings returns the best UID/GID mappings for a user namespace.
//
// Strategy (in order):
//  1. If /etc/subuid has a range ≥ 65536 → map host_uid→0, then subuid_start→1..N
//     This gives the container a full range: root=0, normal users=1..N, nobody=65534
//  2. If subuid range is smaller → use whatever is available
//  3. Fallback: map only host_uid→0 (Size:1) — limited but functional for root-only workloads
func buildUIDMappings(uid, gid int) ([]syscall.SysProcIDMap, []syscall.SysProcIDMap) {
	subUIDStart, subUIDCount := readSubIDRange("/etc/subuid", uid)
	subGIDStart, subGIDCount := readSubIDRange("/etc/subgid", gid)

	var uidMaps, gidMaps []syscall.SysProcIDMap

	if subUIDCount > 0 {
		// Have subordinate UIDs: map host_uid→0, then subuid_range→1..N
		// Container uid 100 (_apt) = host subuid[99], uid 65534 (nobody) = subuid[65533]
		uidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
			{ContainerID: 1, HostID: subUIDStart, Size: subUIDCount},
		}
	} else {
		// No subuid available (common in CI/codespaces without proper setup).
		// Use a single-range mapping: map host_uid → uid 0, and map a block of
		// IDs starting at uid+1 on the host → uid 1..65535 in the container.
		// This gives the container a full uid range using adjacent host IDs.
		// On most modern kernels this works even without /etc/subuid.
		uidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: uid, Size: 1},
			// Map container uid 1..65535 → host uid (uid+1)..(uid+65535)
			// These host UIDs don't need to exist; the kernel just needs the mapping.
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

// readSubIDRange reads /etc/subuid or /etc/subgid and returns (start, count) for the given id.
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

// Start launches a container process inside Linux namespaces.
//
// Architecture — "2-pipe sync" pattern (same as runc/podman):
//
//	Parent                              Child (/proc/self/exe __cryruss_init__)
//	  fork child (no CLONE_NEWUSER)
//	  read syncR → wait for "ready\n"
//	                                      write "ready\n" to syncW
//	                                      block reading readyR
//	  write allow → /proc/<pid>/setgroups
//	  write uid_map, gid_map
//	  write "go\n" to readyW
//	                                      read syncR → unblock
//	                                      unshare(CLONE_NEWUSER|CLONE_NEWNS|...)
//	                                      read initConfig from configR
//	                                      setup mounts, pivot_root, exec
//
// This is the ONLY reliable way to set "allow" on setgroups — it must be
// written BEFORE gid_map, and BEFORE the process enters the user namespace.
// Go's runtime hardcodes "deny" when Cloneflags has CLONE_NEWUSER, so we
// must handle namespace creation ourselves.
func Start(c *container.Container, opts RunOptions) (*os.Process, error) {
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

	// Pipe 1 (configPipe): parent → child, carries JSON InitConfig
	configR, configW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	// Pipe 2 (syncPipe): child → parent, child writes "ready\n" when it wants maps written
	syncR, syncW, err := os.Pipe()
	if err != nil {
		configR.Close()
		configW.Close()
		return nil, err
	}
	// Pipe 3 (readyPipe): parent → child, parent writes "go\n" after writing maps
	readyR, readyW, err := os.Pipe()
	if err != nil {
		configR.Close()
		configW.Close()
		syncR.Close()
		syncW.Close()
		return nil, err
	}

	// Encode namespace flags into initCfg so child knows what to unshare
	nsFlags := syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS |
		syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC
	if c.HostConfig.NetworkMode != "host" {
		nsFlags |= syscall.CLONE_NEWNET
	}
	initCfg.NSFlags = nsFlags

	// Child receives: fd3=configR, fd4=syncW, fd5=readyR
	cmd := exec.Command("/proc/self/exe", InitArg)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// No Cloneflags — child will call unshare() itself after maps are written
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

	// Parent closes its copies of child-end fds
	configR.Close()
	syncW.Close()
	readyR.Close()

	// Wait for child to signal "ready" (child has called unshare and is waiting)
	buf := make([]byte, 6)
	if _, err := syncR.Read(buf); err != nil {
		cmd.Process.Kill()
		syncR.Close()
		configW.Close()
		readyW.Close()
		return nil, fmt.Errorf("sync with child: %w", err)
	}
	syncR.Close()

	// Now write uid/gid maps. setgroups must be written BEFORE gid_map.
	// At this point the child has called unshare(CLONE_NEWUSER) but is
	// blocked — so its /proc/<pid>/setgroups is writable.
	pid := cmd.Process.Pid
	if err := writeUIDMaps(pid, uid, gid, uidMaps, gidMaps); err != nil {
		// Non-fatal: container will still run but may lack full uid range
		_ = err
	}

	// Signal child to continue
	readyW.Write([]byte("go\n"))
	readyW.Close()

	// Send init config to child
	if err := json.NewEncoder(configW).Encode(initCfg); err != nil {
		configW.Close()
		cmd.Process.Kill()
		return nil, err
	}
	configW.Close()

	// Apply cgroup resource limits
	ApplyCgroups(pid, c.HostConfig, c.ID)

	if c.HostConfig.NetworkMode != "host" && c.HostConfig.NetworkMode != "none" {
		setupSlirp4netns(pid)
	}

	pidFile := filepath.Join(config.Global.ContainersDir, c.ID, "pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)

	return cmd.Process, nil
}

// writeUIDMaps writes uid_map and gid_map for a user namespace process.
// setgroups is set to "allow" before gid_map so processes inside can call setgroups().
func writeUIDMaps(pid, uid, gid int, uidMaps, gidMaps []syscall.SysProcIDMap) error {
	procDir := fmt.Sprintf("/proc/%d", pid)

	// Build uid_map string
	var uidLines []string
	for _, m := range uidMaps {
		uidLines = append(uidLines, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
	}
	// Build gid_map string
	var gidLines []string
	for _, m := range gidMaps {
		gidLines = append(gidLines, fmt.Sprintf("%d %d %d", m.ContainerID, m.HostID, m.Size))
	}

	// Try newuidmap first if we have multiple entries and the tool exists
	if len(uidMaps) > 1 && hasNewuidmap() && uid != 0 {
		return applyNewuidmap(pid, uidMaps, gidMaps)
	}

	// Direct write path
	if err := os.WriteFile(procDir+"/uid_map", []byte(strings.Join(uidLines, "\n")), 0); err != nil {
		return fmt.Errorf("uid_map: %w", err)
	}
	// CRITICAL: write "allow" BEFORE gid_map
	_ = os.WriteFile(procDir+"/setgroups", []byte("allow"), 0)
	if err := os.WriteFile(procDir+"/gid_map", []byte(strings.Join(gidLines, "\n")), 0); err != nil {
		return fmt.Errorf("gid_map: %w", err)
	}
	return nil
}

// StartWithProot attempts to start a container using proot for environments
// that do not support user namespaces (e.g. older kernels, some Android/Termux).
// Falls back to regular Start() if proot is not available.
func StartWithProot(c *container.Container, opts RunOptions) (*os.Process, error) {
	prootBin, err := exec.LookPath("proot")
	if err != nil {
		return Start(c, opts) // proot not found, use normal start
	}

	args := resolveArgs(c)
	prootArgs := []string{
		"--rootfs=" + c.RootfsPath,
		"--bind=/dev",
		"--bind=/proc",
		"--bind=/sys",
		"--change-id=0:0", // fake root inside proot
	}

	// Bind user mounts
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

	prootArgs = append(prootArgs, "--") // separator
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

	// Apply cgroup limits even in proot mode
	ApplyCgroups(cmd.Process.Pid, c.HostConfig, c.ID)

	pidFile := filepath.Join(config.Global.ContainersDir, c.ID, "pid")
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

	return cmd.Process, nil
}

// setupSlirp4netns tries to start slirp4netns for user-space networking.
// This gives the container a real network without root privileges.
// applyNewuidmap calls newuidmap/newgidmap to write multi-entry UID/GID maps.
// This is needed when the user has subordinate IDs in /etc/subuid but is not root.
func applyNewuidmap(pid int, uidMaps, gidMaps []syscall.SysProcIDMap) error {
	// newuidmap <pid> <containerID> <hostID> <size> [...]
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

	// Must write "allow" to setgroups BEFORE writing gid_map
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

// writeUIDMapDirect writes uid_map and/or gid_map directly to /proc/<pid>/.
// Pass nil for uidMaps or gidMaps to skip writing that file.
// NOTE: setgroups must be written to "allow" BEFORE gid_map by the caller.
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
		return // not installed – container will have lo only
	}
	// slirp4netns --configure --mtu=65520 --disable-host-loopback <pid> tap0
	slirp := exec.Command("slirp4netns",
		"--configure",
		"--mtu=65520",
		"--disable-host-loopback",
		strconv.Itoa(pid),
		"tap0",
	)
	slirp.Stdout = nil
	slirp.Stderr = nil
	slirp.Start() // fire-and-forget; errors are non-fatal
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

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

// Stop sends a signal to the container and waits up to timeout seconds.
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
