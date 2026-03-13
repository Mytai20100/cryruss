package image

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type RegistryClient struct {
	http     *http.Client
	username string
	password string
}

type authToken struct {
	Token string `json:"token"`
}

func NewRegistryClient(username, password string) *RegistryClient {
	return &RegistryClient{
		http:     &http.Client{Timeout: 120 * time.Second},
		username: username,
		password: password,
	}
}

func (c *RegistryClient) getToken(ref Reference, scope string) (string, error) {
	if ref.Registry != "registry-1.docker.io" {
		return "", nil
	}
	url := fmt.Sprintf("%s/token?service=registry.docker.io&scope=%s", ref.AuthURL(), scope)
	req, _ := http.NewRequest("GET", url, nil)
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var t authToken
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	return t.Token, nil
}

func (c *RegistryClient) fetchManifest(ref Reference, token string) (*Manifest, string, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", ref.RegistryURL(), ref.Name, ref.TagOrDigest())

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("manifest fetch failed %d: %s", resp.StatusCode, string(body))
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	ct := resp.Header.Get("Content-Type")

	

	if strings.Contains(ct, "manifest.list") || strings.Contains(ct, "image.index") {
		var idx ManifestIndex
		if err := json.Unmarshal(body, &idx); err != nil {
			return nil, "", err
		}
		d, err := c.selectPlatform(idx)
		if err != nil {
			return nil, "", err
		}
		ref2 := ref
		ref2.Tag = ""
		ref2.Digest = d
		return c.fetchManifest(ref2, token)
	}

	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, "", err
	}
	return &m, digest, nil
}

func (c *RegistryClient) selectPlatform(idx ManifestIndex) (string, error) {
	arch := runtime.GOARCH
	goos := runtime.GOOS
	if arch == "amd64" {
		arch = "amd64"
	}
	

	for _, e := range idx.Manifests {
		if e.Platform.OS == goos && e.Platform.Architecture == arch {
			return e.Digest, nil
		}
	}
	

	for _, e := range idx.Manifests {
		if e.Platform.OS == "linux" && e.Platform.Architecture == "amd64" {
			return e.Digest, nil
		}
	}
	if len(idx.Manifests) > 0 {
		return idx.Manifests[0].Digest, nil
	}
	return "", fmt.Errorf("no suitable platform found")
}

func (c *RegistryClient) fetchConfig(ref Reference, token, digest string) (*ImageConfigFile, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", ref.RegistryURL(), ref.Name, digest)
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var cfg ImageConfigFile
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

type PullProgress struct {
	Layer   string
	Current int64
	Total   int64
	Done    bool
}

func (c *RegistryClient) downloadLayer(ref Reference, token, digest, destDir string, progress func(PullProgress)) error {
	

	layerDir := filepath.Join(destDir, strings.ReplaceAll(digest, ":", "-"))
	if _, err := os.Stat(layerDir); err == nil {
		return nil
	}

	url := fmt.Sprintf("%s/v2/%s/blobs/%s", ref.RegistryURL(), ref.Name, digest)
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("layer download failed: %d", resp.StatusCode)
	}

	total := resp.ContentLength
	short := digest
	if len(short) > 19 {
		short = short[7:19]
	}

	pr := &progressReader{
		r:     resp.Body,
		total: total,
		fn: func(cur, tot int64) {
			if progress != nil {
				progress(PullProgress{Layer: short, Current: cur, Total: tot})
			}
		},
	}

	os.MkdirAll(layerDir, 0755)
	if err := extractLayer(pr, layerDir); err != nil {
		os.RemoveAll(layerDir)
		return fmt.Errorf("extracting layer %s: %w", short, err)
	}

	if progress != nil {
		progress(PullProgress{Layer: short, Current: total, Total: total, Done: true})
	}
	return nil
}

func extractLayer(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		

		base := filepath.Base(hdr.Name)
		dir := filepath.Dir(hdr.Name)

		if strings.HasPrefix(base, ".wh..wh..opq") {
			

			target := filepath.Join(dest, dir)
			entries, _ := os.ReadDir(target)
			for _, e := range entries {
				os.RemoveAll(filepath.Join(target, e.Name()))
			}
			continue
		}
		if strings.HasPrefix(base, ".wh.") {
			

			target := filepath.Join(dest, dir, strings.TrimPrefix(base, ".wh."))
			os.RemoveAll(target)
			continue
		}

		target := filepath.Join(dest, hdr.Name)
		

		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, os.FileMode(hdr.Mode)|0111)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				continue
			}
			io.Copy(f, tr)
			f.Close()
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(target), 0755)
			os.Remove(target)
			os.Symlink(hdr.Linkname, target)
		case tar.TypeLink:
			os.MkdirAll(filepath.Dir(target), 0755)
			linkTarget := filepath.Join(dest, hdr.Linkname)
			os.Remove(target)
			os.Link(linkTarget, target)
		case tar.TypeChar, tar.TypeBlock:
			

			continue
		case tar.TypeFifo:
			os.MkdirAll(filepath.Dir(target), 0755)
			

		}
	}
	return nil
}

type progressReader struct {
	r       io.Reader
	current int64
	total   int64
	fn      func(int64, int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.current += int64(n)
	if p.fn != nil {
		p.fn(p.current, p.total)
	}
	return n, err
}

func (c *RegistryClient) Pull(rawRef string, imagesDir string, progress func(PullProgress)) (*Image, *ImageConfigFile, []string, error) {
	ref := ParseReference(rawRef)
	scope := fmt.Sprintf("repository:%s:pull", ref.Name)

	token, err := c.getToken(ref, scope)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("auth: %w", err)
	}

	manifest, digest, err := c.fetchManifest(ref, token)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("manifest: %w", err)
	}

	imgCfg, err := c.fetchConfig(ref, token, manifest.Config.Digest)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("config: %w", err)
	}

	layerDir := filepath.Join(imagesDir, "layers")
	os.MkdirAll(layerDir, 0755)

	var layerPaths []string
	for _, layer := range manifest.Layers {
		if err := c.downloadLayer(ref, token, layer.Digest, layerDir, progress); err != nil {
			return nil, nil, nil, fmt.Errorf("layer %s: %w", layer.Digest, err)
		}
		lp := filepath.Join(layerDir, strings.ReplaceAll(layer.Digest, ":", "-"))
		layerPaths = append(layerPaths, lp)
	}

	imgID := strings.TrimPrefix(manifest.Config.Digest, "sha256:")
	

	var size int64
	for _, l := range manifest.Layers {
		size += l.Size
	}

	img := &Image{
		ID:          "sha256:" + imgID,
		RepoTags:    []string{ref.String()},
		RepoDigests: []string{ref.Name + "@" + digest},
		Created:     imgCfg.Created.Unix(),
		Size:        size,
		VirtualSize: size,
		Labels:      imgCfg.Config.Labels,
		Digest:      digest,
		Layers:      layerPaths,
		Config: &ImageConfig{
			Hostname:     imgCfg.Config.Hostname,
			User:         imgCfg.Config.User,
			Env:          imgCfg.Config.Env,
			Cmd:          imgCfg.Config.Cmd,
			Entrypoint:   imgCfg.Config.Entrypoint,
			WorkingDir:   imgCfg.Config.WorkingDir,
			Labels:       imgCfg.Config.Labels,
			ExposedPorts: imgCfg.Config.ExposedPorts,
			Volumes:      imgCfg.Config.Volumes,
			StopSignal:   imgCfg.Config.StopSignal,
		},
		RootFS: RootFS{
			Type:   imgCfg.RootFS.Type,
			Layers: imgCfg.RootFS.DiffIDs,
		},
	}
	if img.Config.Labels == nil {
		img.Config.Labels = map[string]string{}
	}

	return img, imgCfg, layerPaths, nil
}
