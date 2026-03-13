package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	cryruss "github.com/cryruss/cryruss"
	"github.com/cryruss/cryruss/pkg/container"
	"github.com/cryruss/cryruss/pkg/image"
	"github.com/cryruss/cryruss/pkg/network"
	rt "github.com/cryruss/cryruss/pkg/runtime"
	"github.com/cryruss/cryruss/pkg/volume"
)

type Handlers struct {
	containers *container.Manager
	images     *image.Manager
	networks   *network.Manager
	volumes    *volume.Manager
}

func New() *Handlers {
	return &Handlers{
		containers: container.NewManager(),
		images:     image.NewManager(),
		networks:   network.NewManager(),
		volumes:    volume.NewManager(),
	}
}

// ---- System ----

func (h *Handlers) Ping(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

func (h *Handlers) Version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"Version":       cryruss.Version,
		"ApiVersion":    cryruss.APIVersion,
		"MinAPIVersion": cryruss.MinAPIVersion,
		"GoVersion":     runtime.Version(),
		"Os":            runtime.GOOS,
		"Arch":          runtime.GOARCH,
		"BuildTime":     time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handlers) Info(w http.ResponseWriter, r *http.Request) {
	containers, _ := h.containers.List(true)
	images, _ := h.images.List()
	running := 0
	stopped := 0
	paused := 0
	for _, c := range containers {
		switch c.State.Status {
		case container.StatusRunning:
			running++
		case container.StatusPaused:
			paused++
		default:
			stopped++
		}
	}
	writeJSON(w, 200, map[string]any{
		"ID":                "cryruss0",
		"Containers":        len(containers),
		"ContainersRunning": running,
		"ContainersPaused":  paused,
		"ContainersStopped": stopped,
		"Images":            len(images),
		"Driver":            "overlay2",
		"MemoryLimit":       true,
		"SwapLimit":         true,
		"KernelMemory":      false,
		"CpuCfsPeriod":      true,
		"CpuCfsQuota":       true,
		"IPv4Forwarding":    true,
		"BridgeNfIptables":  false,
		"OomKillDisable":    false,
		"NGoroutines":       runtime.NumGoroutine(),
		"LoggingDriver":     "json-file",
		"CgroupDriver":      "none",
		"DockerRootDir":     os.Getenv("HOME") + "/.local/share/cryruss",
		"Name":              "cryruss",
		"ServerVersion":     cryruss.Version,
		"OperatingSystem":   runtime.GOOS,
		"Architecture":      runtime.GOARCH,
		"NCPU":              runtime.NumCPU(),
	})
}

// ---- Containers ----

func (h *Handlers) ContainerList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	containers, err := h.containers.List(all)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	// Refresh running state
	for _, c := range containers {
		if c.State.Running {
			pid := rt.GetPID(c.ID)
			if pid > 0 && !rt.IsRunning(pid) {
				c.State.Running = false
				c.State.Status = container.StatusExited
				c.State.FinishedAt = time.Now()
				h.containers.Save(c)
			}
		}
		c.Status = h.containers.StatusString(c)
	}
	if containers == nil {
		containers = []*container.Container{}
	}
	writeJSON(w, 200, containers)
}

func (h *Handlers) ContainerCreate(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	var req container.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	c, err := h.containers.Create(&req, name)
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"Id": c.ID, "Warnings": []string{}})
}

func (h *Handlers) ContainerInspect(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	c.Status = h.containers.StatusString(c)
	writeJSON(w, 200, c)
}

func (h *Handlers) ContainerStart(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if c.State.Running {
		w.WriteHeader(304)
		return
	}

	// Setup rootfs if needed
	if !hasRootfs(c.RootfsPath) {
		img, err := h.images.Get(c.Config.Image)
		if err != nil {
			writeError(w, 404, "image not found: "+c.Config.Image)
			return
		}
		if err := h.images.PrepareRootfs(img, c.RootfsPath); err != nil {
			writeError(w, 500, "rootfs: "+err.Error())
			return
		}
	}

	proc, err := rt.Start(c, rt.RunOptions{Detach: true})
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	h.containers.UpdateState(c.ID, func(c *container.Container) {
		c.State.Status = container.StatusRunning
		c.State.Running = true
		c.State.Pid = proc.Pid
		c.State.StartedAt = time.Now()
	})

	// Watch in background
	go func() {
		proc.Wait()
		h.containers.UpdateState(c.ID, func(c *container.Container) {
			c.State.Running = false
			c.State.Status = container.StatusExited
			c.State.FinishedAt = time.Now()
			c.State.Pid = 0
		})
	}()

	w.WriteHeader(204)
}

func (h *Handlers) ContainerStop(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if !c.State.Running {
		w.WriteHeader(304)
		return
	}
	timeout := 10
	if t := r.URL.Query().Get("t"); t != "" {
		if v, err := strconv.Atoi(t); err == nil {
			timeout = v
		}
	}
	rt.Stop(c, syscall.SIGTERM, timeout)
	h.containers.UpdateState(c.ID, func(c *container.Container) {
		c.State.Running = false
		c.State.Status = container.StatusStopped
		c.State.FinishedAt = time.Now()
		c.State.Pid = 0
	})
	w.WriteHeader(204)
}

func (h *Handlers) ContainerRestart(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if c.State.Running {
		rt.Stop(c, syscall.SIGTERM, 10)
	}
	// Re-use ContainerStart logic
	h.containers.UpdateState(c.ID, func(c *container.Container) {
		c.State.Running = false
		c.State.Status = container.StatusStopped
	})
	proc, err := rt.Start(c, rt.RunOptions{Detach: true})
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	h.containers.UpdateState(c.ID, func(c *container.Container) {
		c.State.Running = true
		c.State.Status = container.StatusRunning
		c.State.Pid = proc.Pid
		c.State.StartedAt = time.Now()
	})
	go func() {
		proc.Wait()
		h.containers.UpdateState(id, func(c *container.Container) {
			c.State.Running = false
			c.State.Status = container.StatusExited
			c.State.FinishedAt = time.Now()
		})
	}()
	w.WriteHeader(204)
}

func (h *Handlers) ContainerKill(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	sig := syscall.SIGKILL
	if s := r.URL.Query().Get("signal"); s != "" {
		if s == "SIGTERM" {
			sig = syscall.SIGTERM
		} else if s == "SIGINT" {
			sig = syscall.SIGINT
		}
	}
	rt.Stop(c, sig, 0)
	w.WriteHeader(204)
}

func (h *Handlers) ContainerRemove(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if c.State.Running && !force {
		writeError(w, 409, "container is running; stop it first or use --force")
		return
	}
	if c.State.Running {
		rt.Stop(c, syscall.SIGKILL, 5)
	}
	if err := h.containers.Delete(id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handlers) ContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	tail := r.URL.Query().Get("tail")
	f, err := os.Open(c.LogPath)
	if err != nil {
		w.WriteHeader(200)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(200)

	if tail == "all" || tail == "" {
		io.Copy(w, f)
	} else {
		n, _ := strconv.Atoi(tail)
		if n <= 0 {
			n = 100
		}
		tailFile(w, f, n)
	}
}

func tailFile(w io.Writer, f *os.File, n int) {
	fi, err := f.Stat()
	if err != nil {
		return
	}
	size := fi.Size()
	if size == 0 {
		return
	}
	// Simple approach: read all, take last n lines
	b, err := io.ReadAll(f)
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	fmt.Fprint(w, strings.Join(lines, "\n"))
}

func (h *Handlers) ContainerExec(w http.ResponseWriter, r *http.Request) {
	writeError(w, 501, "exec not supported via API; use cryruss exec CLI")
}

func (h *Handlers) ContainerStats(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"id":   c.ID,
		"name": strings.Join(c.Names, ","),
		"cpu_stats": map[string]any{
			"cpu_usage": map[string]any{"total_usage": 0},
		},
		"memory_stats": map[string]any{"usage": 0, "limit": 0},
		"networks":     map[string]any{},
	})
}

func (h *Handlers) ContainerTop(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	_, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"Titles":    []string{"PID", "USER", "TIME", "COMMAND"},
		"Processes": [][]string{},
	})
}

func (h *Handlers) ContainerPrune(w http.ResponseWriter, r *http.Request) {
	containers, _ := h.containers.List(true)
	var deleted []string
	var freed int64
	for _, c := range containers {
		if !c.State.Running {
			h.containers.Delete(c.ID)
			deleted = append(deleted, c.ID)
		}
	}
	writeJSON(w, 200, map[string]any{
		"ContainersDeleted": deleted,
		"SpaceReclaimed":    freed,
	})
}

func (h *Handlers) ContainerRename(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	newName := r.URL.Query().Get("name")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if !strings.HasPrefix(newName, "/") {
		newName = "/" + newName
	}
	c.Names = []string{newName}
	h.containers.Save(c)
	w.WriteHeader(204)
}

func (h *Handlers) ContainerPause(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if c.State.Pid > 0 {
		proc, _ := os.FindProcess(c.State.Pid)
		if proc != nil {
			proc.Signal(syscall.SIGSTOP)
		}
	}
	h.containers.UpdateState(id, func(c *container.Container) {
		c.State.Paused = true
		c.State.Status = container.StatusPaused
	})
	w.WriteHeader(204)
}

func (h *Handlers) ContainerUnpause(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	c, err := h.containers.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	if c.State.Pid > 0 {
		proc, _ := os.FindProcess(c.State.Pid)
		if proc != nil {
			proc.Signal(syscall.SIGCONT)
		}
	}
	h.containers.UpdateState(id, func(c *container.Container) {
		c.State.Paused = false
		c.State.Status = container.StatusRunning
	})
	w.WriteHeader(204)
}

func (h *Handlers) ContainerChanges(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, []any{})
}

// ---- Images ----

func (h *Handlers) ImageList(w http.ResponseWriter, r *http.Request) {
	images, err := h.images.List()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if images == nil {
		images = []*image.Image{}
	}
	writeJSON(w, 200, images)
}

func (h *Handlers) ImageCreate(w http.ResponseWriter, r *http.Request) {
	fromImage := r.URL.Query().Get("fromImage")
	tag := r.URL.Query().Get("tag")
	ref := fromImage
	if tag != "" && !strings.Contains(fromImage, ":") {
		ref = fromImage + ":" + tag
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	enc := json.NewEncoder(w)
	img, err := h.images.Pull(ref, func(p image.PullProgress) {
		if p.Done {
			enc.Encode(map[string]any{"status": "Pull complete", "id": p.Layer})
		} else if p.Total > 0 {
			enc.Encode(map[string]any{
				"status": "Downloading",
				"id":     p.Layer,
				"progressDetail": map[string]any{
					"current": p.Current,
					"total":   p.Total,
				},
			})
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
	if err != nil {
		enc.Encode(map[string]any{"errorDetail": map[string]string{"message": err.Error()}, "error": err.Error()})
		return
	}
	enc.Encode(map[string]any{"status": "Status: Downloaded newer image for " + ref, "id": image.ShortID(img.ID)})
}

func (h *Handlers) ImageInspect(w http.ResponseWriter, r *http.Request) {
	name := pathParam(r, "name")
	img, err := h.images.Get(name)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, img)
}

func (h *Handlers) ImageRemove(w http.ResponseWriter, r *http.Request) {
	name := pathParam(r, "name")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := h.images.Delete(name, force); err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, []map[string]string{{"Untagged": name}})
}

func (h *Handlers) ImageTag(w http.ResponseWriter, r *http.Request) {
	name := pathParam(r, "name")
	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")
	newTag := repo
	if tag != "" {
		newTag = repo + ":" + tag
	}
	if err := h.images.Tag(name, newTag); err != nil {
		writeError(w, 404, err.Error())
		return
	}
	w.WriteHeader(201)
}

func (h *Handlers) ImagePrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"ImagesDeleted":  []any{},
		"SpaceReclaimed": 0,
	})
}

func (h *Handlers) ImageSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, []any{})
}

// ---- Networks ----

func (h *Handlers) NetworkList(w http.ResponseWriter, r *http.Request) {
	networks, err := h.networks.List()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if networks == nil {
		networks = []*network.Network{}
	}
	writeJSON(w, 200, networks)
}

func (h *Handlers) NetworkCreate(w http.ResponseWriter, r *http.Request) {
	var req network.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	n, err := h.networks.Create(&req)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"Id": n.ID, "Warning": ""})
}

func (h *Handlers) NetworkInspect(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	n, err := h.networks.Get(id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, n)
}

func (h *Handlers) NetworkRemove(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")
	if err := h.networks.Delete(id); err != nil {
		writeError(w, 403, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handlers) NetworkPrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"NetworksDeleted": []string{}})
}

func (h *Handlers) NetworkConnect(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

func (h *Handlers) NetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

// ---- Volumes ----

func (h *Handlers) VolumeList(w http.ResponseWriter, r *http.Request) {
	volumes, err := h.volumes.List()
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if volumes == nil {
		volumes = []*volume.Volume{}
	}
	writeJSON(w, 200, map[string]any{
		"Volumes":  volumes,
		"Warnings": []string{},
	})
}

func (h *Handlers) VolumeCreate(w http.ResponseWriter, r *http.Request) {
	var req volume.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	v, err := h.volumes.Create(&req)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, v)
}

func (h *Handlers) VolumeInspect(w http.ResponseWriter, r *http.Request) {
	name := pathParam(r, "name")
	v, err := h.volumes.Get(name)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, v)
}

func (h *Handlers) VolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := pathParam(r, "name")
	force := r.URL.Query().Get("force") == "1"
	if err := h.volumes.Delete(name, force); err != nil {
		writeError(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handlers) VolumePrune(w http.ResponseWriter, r *http.Request) {
	removed, freed, _ := h.volumes.Prune()
	writeJSON(w, 200, map[string]any{
		"VolumesDeleted": removed,
		"SpaceReclaimed": freed,
	})
}

// ---- Helpers ----

func pathParam(r *http.Request, key string) string {
	path := r.URL.Path
	// Strip version prefix
	if len(path) > 4 && path[0] == '/' {
		parts := strings.SplitN(path[1:], "/", 2)
		if len(parts) == 2 && (strings.HasPrefix(parts[0], "v1.") || strings.HasPrefix(parts[0], "v2.")) {
			path = "/" + parts[1]
		}
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Find {key} position pattern from known routes
	// Simple: take the segment after the resource type
	if key == "id" || key == "name" {
		if len(parts) >= 2 {
			return parts[1]
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"message": msg})
}

func hasRootfs(path string) bool {
	// Check if rootfs has at least /bin or /usr
	_, err1 := os.Stat(filepath.Join(path, "bin"))
	_, err2 := os.Stat(filepath.Join(path, "usr"))
	return err1 == nil || err2 == nil
}
