

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/cryruss/cryruss/pkg/container"
)

func cgroupVersion() int {
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return 2
	}
	return 1
}

func cgroupPath(subsystem, containerID string, version int) string {
	if version == 2 {
		return filepath.Join("/sys/fs/cgroup/cryruss", containerID)
	}
	return filepath.Join("/sys/fs/cgroup", subsystem, "cryruss", containerID)
}

func writeCg(path, value string) error {
	return os.WriteFile(path, []byte(value), 0644)
}

func ApplyCgroups(pid int, hc container.HostConfig, containerID string) {
	if cgroupVersion() == 2 {
		applyCgroupV2(pid, hc, containerID)
	} else {
		applyCgroupV1(pid, hc, containerID)
	}
}

func applyCgroupV2(pid int, hc container.HostConfig, containerID string) {
	base := cgroupPath("", containerID, 2)
	if !canWriteCgroupDir(base) {
		base = userCgroupV2(containerID)
		if base == "" {
			return
		}
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return
	}
	enableV2Controllers(filepath.Dir(base))
	_ = writeCg(filepath.Join(base, "cgroup.procs"), strconv.Itoa(pid))

	

	if hc.Memory > 0 {
		_ = writeCg(filepath.Join(base, "memory.max"), strconv.FormatInt(hc.Memory, 10))
	}
	if hc.MemorySwap > 0 {
		_ = writeCg(filepath.Join(base, "memory.swap.max"), strconv.FormatInt(hc.MemorySwap, 10))
	} else if hc.MemorySwap == -1 {
		_ = writeCg(filepath.Join(base, "memory.swap.max"), "max")
	}
	if hc.MemoryReservation > 0 {
		_ = writeCg(filepath.Join(base, "memory.low"), strconv.FormatInt(hc.MemoryReservation, 10))
	}

	

	period := hc.CPUPeriod
	if period == 0 {
		period = 100000
	}
	if hc.NanoCPUs > 0 {
		quota := hc.NanoCPUs * period / 1e9
		_ = writeCg(filepath.Join(base, "cpu.max"), fmt.Sprintf("%d %d", quota, period))
	} else if hc.CPUQuota > 0 {
		_ = writeCg(filepath.Join(base, "cpu.max"), fmt.Sprintf("%d %d", hc.CPUQuota, period))
	}
	if hc.CPUShares > 0 {
		_ = writeCg(filepath.Join(base, "cpu.weight"), strconv.FormatInt(cpuSharesToWeight(hc.CPUShares), 10))
	}
	if hc.CPUSetCPUs != "" {
		_ = writeCg(filepath.Join(base, "cpuset.cpus"), hc.CPUSetCPUs)
	}
	if hc.CPUSetMems != "" {
		_ = writeCg(filepath.Join(base, "cpuset.mems"), hc.CPUSetMems)
	}

	

	if hc.PidsLimit > 0 {
		_ = writeCg(filepath.Join(base, "pids.max"), strconv.FormatInt(hc.PidsLimit, 10))
	} else if hc.PidsLimit == -1 {
		_ = writeCg(filepath.Join(base, "pids.max"), "max")
	}

	

	type ioEntry struct{ rbps, wbps, riops, wiops int64 }
	ioMap := map[string]*ioEntry{}
	fill := func(devs []container.ThrottleDevice, field string) {
		for _, d := range devs {
			maj, min, err := deviceMajMin(d.Path)
			if err != nil {
				continue
			}
			k := fmt.Sprintf("%d:%d", maj, min)
			if ioMap[k] == nil {
				ioMap[k] = &ioEntry{}
			}
			switch field {
			case "rbps":
				ioMap[k].rbps = d.Rate
			case "wbps":
				ioMap[k].wbps = d.Rate
			case "riops":
				ioMap[k].riops = d.Rate
			case "wiops":
				ioMap[k].wiops = d.Rate
			}
		}
	}
	fill(hc.BlkioDeviceReadBps, "rbps")
	fill(hc.BlkioDeviceWriteBps, "wbps")
	fill(hc.BlkioDeviceReadIOps, "riops")
	fill(hc.BlkioDeviceWriteIOps, "wiops")
	for k, e := range ioMap {
		parts := []string{k}
		if e.rbps > 0 {
			parts = append(parts, fmt.Sprintf("rbps=%d", e.rbps))
		}
		if e.wbps > 0 {
			parts = append(parts, fmt.Sprintf("wbps=%d", e.wbps))
		}
		if e.riops > 0 {
			parts = append(parts, fmt.Sprintf("riops=%d", e.riops))
		}
		if e.wiops > 0 {
			parts = append(parts, fmt.Sprintf("wiops=%d", e.wiops))
		}
		_ = writeCg(filepath.Join(base, "io.max"), strings.Join(parts, " "))
	}
	if hc.BlkioWeight > 0 {
		_ = writeCg(filepath.Join(base, "io.weight"), fmt.Sprintf("default %d", hc.BlkioWeight))
	}
}

func enableV2Controllers(parent string) {
	for _, ctrl := range []string{"memory", "cpu", "cpuset", "io", "pids"} {
		_ = writeCg(filepath.Join(parent, "cgroup.subtree_control"), "+"+ctrl)
	}
}

func userCgroupV2(containerID string) string {
	uid := os.Getuid()
	for _, p := range []string{
		filepath.Join("/sys/fs/cgroup/user.slice", fmt.Sprintf("user-%d.slice", uid), "cryruss", containerID),
		filepath.Join("/sys/fs/cgroup/user.slice", "cryruss", containerID),
	} {
		if _, err := os.Stat(filepath.Dir(p)); err == nil {
			return p
		}
	}
	return ""
}

func canWriteCgroupDir(path string) bool {
	parent := filepath.Dir(path)
	test := filepath.Join(parent, ".cryruss_wr")
	if err := os.WriteFile(test, []byte(""), 0644); err != nil {
		return false
	}
	os.Remove(test)
	return true
}

func applyCgroupV1(pid int, hc container.HostConfig, containerID string) {
	type setup struct {
		sub string
		fn  func(string)
	}
	var setups []setup

	

	if hc.Memory > 0 || hc.MemorySwap != 0 || hc.MemoryReservation > 0 || hc.KernelMemory > 0 {
		setups = append(setups, setup{"memory", func(b string) {
			if hc.Memory > 0 {
				_ = writeCg(filepath.Join(b, "memory.limit_in_bytes"), strconv.FormatInt(hc.Memory, 10))
			}
			if hc.MemorySwap > 0 {
				_ = writeCg(filepath.Join(b, "memory.memsw.limit_in_bytes"), strconv.FormatInt(hc.MemorySwap, 10))
			} else if hc.MemorySwap == -1 {
				_ = writeCg(filepath.Join(b, "memory.memsw.limit_in_bytes"), "-1")
			}
			if hc.MemoryReservation > 0 {
				_ = writeCg(filepath.Join(b, "memory.soft_limit_in_bytes"), strconv.FormatInt(hc.MemoryReservation, 10))
			}
			if hc.MemorySwappiness >= 0 && hc.MemorySwappiness <= 100 {
				_ = writeCg(filepath.Join(b, "memory.swappiness"), strconv.FormatInt(hc.MemorySwappiness, 10))
			}
			if hc.KernelMemory > 0 {
				_ = writeCg(filepath.Join(b, "memory.kmem.limit_in_bytes"), strconv.FormatInt(hc.KernelMemory, 10))
			}
		}})
	}

	

	if hc.NanoCPUs > 0 || hc.CPUShares > 0 || hc.CPUQuota > 0 {
		setups = append(setups, setup{"cpu", func(b string) {
			if hc.CPUShares > 0 {
				_ = writeCg(filepath.Join(b, "cpu.shares"), strconv.FormatInt(hc.CPUShares, 10))
			}
			period := hc.CPUPeriod
			if period == 0 {
				period = 100000
			}
			_ = writeCg(filepath.Join(b, "cpu.cfs_period_us"), strconv.FormatInt(period, 10))
			if hc.NanoCPUs > 0 {
				_ = writeCg(filepath.Join(b, "cpu.cfs_quota_us"), strconv.FormatInt(hc.NanoCPUs*period/1e9, 10))
			} else if hc.CPUQuota > 0 {
				_ = writeCg(filepath.Join(b, "cpu.cfs_quota_us"), strconv.FormatInt(hc.CPUQuota, 10))
			}
		}})
	}

	

	if hc.CPUSetCPUs != "" || hc.CPUSetMems != "" {
		setups = append(setups, setup{"cpuset", func(b string) {
			if hc.CPUSetCPUs != "" {
				_ = writeCg(filepath.Join(b, "cpuset.cpus"), hc.CPUSetCPUs)
			} else if data, err := os.ReadFile("/sys/fs/cgroup/cpuset/cpuset.cpus"); err == nil {
				_ = writeCg(filepath.Join(b, "cpuset.cpus"), strings.TrimSpace(string(data)))
			}
			if hc.CPUSetMems != "" {
				_ = writeCg(filepath.Join(b, "cpuset.mems"), hc.CPUSetMems)
			} else if data, err := os.ReadFile("/sys/fs/cgroup/cpuset/cpuset.mems"); err == nil {
				_ = writeCg(filepath.Join(b, "cpuset.mems"), strings.TrimSpace(string(data)))
			}
		}})
	}

	

	if hc.PidsLimit != 0 {
		setups = append(setups, setup{"pids", func(b string) {
			if hc.PidsLimit == -1 {
				_ = writeCg(filepath.Join(b, "pids.max"), "max")
			} else {
				_ = writeCg(filepath.Join(b, "pids.max"), strconv.FormatInt(hc.PidsLimit, 10))
			}
		}})
	}

	

	if hc.BlkioWeight > 0 || len(hc.BlkioWeightDevice) > 0 ||
		len(hc.BlkioDeviceReadBps) > 0 || len(hc.BlkioDeviceWriteBps) > 0 ||
		len(hc.BlkioDeviceReadIOps) > 0 || len(hc.BlkioDeviceWriteIOps) > 0 {
		setups = append(setups, setup{"blkio", func(b string) {
			if hc.BlkioWeight > 0 {
				_ = writeCg(filepath.Join(b, "blkio.weight"), strconv.Itoa(int(hc.BlkioWeight)))
			}
			for _, d := range hc.BlkioWeightDevice {
				maj, min, err := deviceMajMin(d.Path)
				if err != nil {
					continue
				}
				_ = writeCg(filepath.Join(b, "blkio.weight_device"), fmt.Sprintf("%d:%d %d", maj, min, d.Weight))
			}
			wt := func(file string, devs []container.ThrottleDevice) {
				for _, d := range devs {
					maj, min, err := deviceMajMin(d.Path)
					if err != nil {
						continue
					}
					_ = writeCg(filepath.Join(b, file), fmt.Sprintf("%d:%d %d", maj, min, d.Rate))
				}
			}
			wt("blkio.throttle.read_bps_device", hc.BlkioDeviceReadBps)
			wt("blkio.throttle.write_bps_device", hc.BlkioDeviceWriteBps)
			wt("blkio.throttle.read_iops_device", hc.BlkioDeviceReadIOps)
			wt("blkio.throttle.write_iops_device", hc.BlkioDeviceWriteIOps)
		}})
	}

	for _, s := range setups {
		base := cgroupPath(s.sub, containerID, 1)
		if err := os.MkdirAll(base, 0755); err != nil {
			continue 

		}
		_ = writeCg(filepath.Join(base, "tasks"), strconv.Itoa(pid))
		s.fn(base)
	}
}

func CleanupCgroups(containerID string) {
	if cgroupVersion() == 2 {
		os.Remove(cgroupPath("", containerID, 2))
		if alt := userCgroupV2(containerID); alt != "" {
			os.Remove(alt)
		}
		return
	}
	for _, sub := range []string{"memory", "cpu", "cpuset", "pids", "blkio"} {
		os.Remove(cgroupPath(sub, containerID, 1))
	}
}

func ReadCgroupStats(containerID string) map[string]int64 {
	stats := map[string]int64{}
	if cgroupVersion() == 2 {
		base := cgroupPath("", containerID, 2)
		if _, err := os.Stat(base); err != nil {
			if alt := userCgroupV2(containerID); alt != "" {
				base = alt
			}
		}
		readInt64 := func(file string, key string) {
			if data, err := os.ReadFile(filepath.Join(base, file)); err == nil {
				var v int64
				fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &v)
				stats[key] = v
			}
		}
		readInt64("memory.current", "memory_usage")
		readInt64("pids.current", "pids_current")
		if data, err := os.ReadFile(filepath.Join(base, "cpu.stat")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if parts := strings.Fields(line); len(parts) == 2 && parts[0] == "usage_usec" {
					var v int64
					fmt.Sscanf(parts[1], "%d", &v)
					stats["cpu_usage_usec"] = v
				}
			}
		}
	} else {
		readInt64 := func(sub, file, key string) {
			path := filepath.Join(cgroupPath(sub, containerID, 1), file)
			if data, err := os.ReadFile(path); err == nil {
				var v int64
				fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &v)
				stats[key] = v
			}
		}
		readInt64("memory", "memory.usage_in_bytes", "memory_usage")
		readInt64("cpuacct", "cpuacct.usage", "cpu_usage_ns")
		readInt64("pids", "pids.current", "pids_current")
	}
	return stats
}

func cpuSharesToWeight(shares int64) int64 {
	if shares <= 0 {
		return 100
	}
	w := int64(1) + ((shares-2)*9999)/262142
	if w < 1 {
		w = 1
	}
	if w > 10000 {
		w = 10000
	}
	return w
}

func deviceMajMin(path string) (uint64, uint64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("cannot get stat for %s", path)
	}
	rdev := st.Rdev
	maj := uint64(rdev>>8) & 0xfff
	min := uint64(rdev&0xff) | uint64((rdev>>12)&0xfff00)
	return maj, min, nil
}
