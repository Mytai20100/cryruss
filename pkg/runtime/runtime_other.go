

package runtime

import (
	"fmt"
	"os"
	"syscall"

	"github.com/cryruss/cryruss/pkg/container"
)

const InitArg = "__cryruss_init__"

func ContainerInit() {
	fmt.Fprintln(os.Stderr, "cryruss: container runtime only supported on Linux")
	os.Exit(1)
}

type RunOptions struct {
	Detach      bool
	Interactive bool
	TTY         bool
}

func Start(c *container.Container, opts RunOptions) (*os.Process, error) {
	return nil, fmt.Errorf("container runtime only supported on Linux")
}

func Stop(c *container.Container, signal syscall.Signal, timeout int) error {
	return fmt.Errorf("container runtime only supported on Linux")
}

func GetPID(containerID string) int {
	return 0
}

func IsRunning(pid int) bool {
	return false
}

func StartWithProot(c *container.Container, opts RunOptions) (*os.Process, error) {
	return nil, fmt.Errorf("container runtime only supported on Linux")
}

func ApplyCgroups(pid int, hc container.HostConfig, containerID string) {}

func CleanupCgroups(containerID string) {}

func ReadCgroupStats(containerID string) map[string]int64 {
	return map[string]int64{}
}
