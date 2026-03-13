package image

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryruss/cryruss/pkg/config"
	"github.com/cryruss/cryruss/pkg/store"
)

type Manager struct {
	store  *store.Store
	client *RegistryClient
}

func NewManager() *Manager {
	return &Manager{
		store:  store.New(config.Global.ImagesDir),
		client: NewRegistryClient("", ""),
	}
}

func (m *Manager) SetCredentials(username, password string) {
	m.client = NewRegistryClient(username, password)
}

func (m *Manager) Pull(rawRef string, progress func(PullProgress)) (*Image, error) {
	img, _, layers, err := m.client.Pull(rawRef, config.Global.ImagesDir, progress)
	if err != nil {
		return nil, err
	}
	img.Layers = layers

	id := strings.TrimPrefix(img.ID, "sha256:")
	if err := m.store.Save(id, img); err != nil {
		return nil, err
	}
	return img, nil
}

func (m *Manager) Get(idOrTag string) (*Image, error) {
	ids, err := m.store.List()
	if err != nil {
		return nil, err
	}

	// Exact ID
	for _, id := range ids {
		if id == idOrTag || "sha256:"+id == idOrTag {
			var img Image
			if err := m.store.Load(id, &img); err != nil {
				return nil, err
			}
			return &img, nil
		}
	}

	// By tag
	for _, id := range ids {
		var img Image
		if err := m.store.Load(id, &img); err != nil {
			continue
		}
		for _, tag := range img.RepoTags {
			if tag == idOrTag {
				return &img, nil
			}
		}
	}

	// Prefix match
	resolved, err := store.ResolveID(ids, idOrTag)
	if err != nil {
		return nil, fmt.Errorf("no such image: %s", idOrTag)
	}
	var img Image
	if err := m.store.Load(resolved, &img); err != nil {
		return nil, err
	}
	return &img, nil
}

func (m *Manager) List() ([]*Image, error) {
	ids, err := m.store.List()
	if err != nil {
		return nil, err
	}
	var result []*Image
	for _, id := range ids {
		var img Image
		if err := m.store.Load(id, &img); err != nil {
			continue
		}
		result = append(result, &img)
	}
	return result, nil
}

func (m *Manager) Delete(idOrTag string, force bool) error {
	img, err := m.Get(idOrTag)
	if err != nil {
		return err
	}
	id := strings.TrimPrefix(img.ID, "sha256:")
	return m.store.Delete(id)
}

func (m *Manager) Tag(src, newTag string) error {
	img, err := m.Get(src)
	if err != nil {
		return err
	}
	// Add new tag if not present
	for _, t := range img.RepoTags {
		if t == newTag {
			return nil
		}
	}
	img.RepoTags = append(img.RepoTags, newTag)
	id := strings.TrimPrefix(img.ID, "sha256:")
	return m.store.Save(id, img)
}

// PrepareRootfs builds container rootfs from image layers
func (m *Manager) PrepareRootfs(img *Image, rootfs string) error {
	os.MkdirAll(rootfs, 0755)

	// Check fuse-overlayfs availability
	if canOverlay(rootfs) {
		return m.overlayRootfs(img, rootfs)
	}
	// Fall back to copy
	return m.copyRootfs(img, rootfs)
}

func canOverlay(rootfs string) bool {
	// Check if fuse-overlayfs is available
	_, err := os.Stat("/usr/bin/fuse-overlayfs")
	if err != nil {
		_, err = lookPath("fuse-overlayfs")
	}
	return err == nil
}

func lookPath(name string) (string, error) {
	paths := []string{"/usr/local/bin", "/usr/bin", "/bin", "/sbin"}
	for _, p := range paths {
		full := filepath.Join(p, name)
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}
	return "", fmt.Errorf("not found: %s", name)
}

func (m *Manager) copyRootfs(img *Image, rootfs string) error {
	for _, layerPath := range img.Layers {
		if err := copyDir(layerPath, rootfs); err != nil {
			return fmt.Errorf("copying layer %s: %w", layerPath, err)
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode()|0111)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			os.Remove(target)
			return os.Symlink(linkTarget, target)
		}

		os.MkdirAll(filepath.Dir(target), 0755)
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return nil
}

func (m *Manager) overlayRootfs(img *Image, rootfs string) error {
	// TODO: implement fuse-overlayfs based overlay
	// For now fall back to copy
	return m.copyRootfs(img, rootfs)
}

func FormatSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f kB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func ShortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
