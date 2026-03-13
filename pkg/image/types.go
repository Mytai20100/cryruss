package image

import "time"

type Image struct {
	ID          string            `json:"Id"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	VirtualSize int64             `json:"VirtualSize"`
	Labels      map[string]string `json:"Labels"`
	Config      *ImageConfig      `json:"Config"`
	RootFS      RootFS            `json:"RootFS"`
	Layers      []string          `json:"Layers"`
	Digest      string            `json:"Digest"`
}

type ImageConfig struct {
	Hostname     string              `json:"Hostname"`
	User         string              `json:"User"`
	Env          []string            `json:"Env"`
	Cmd          []string            `json:"Cmd"`
	Entrypoint   []string            `json:"Entrypoint"`
	WorkingDir   string              `json:"WorkingDir"`
	Labels       map[string]string   `json:"Labels"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Volumes      map[string]struct{} `json:"Volumes"`
	StopSignal   string              `json:"StopSignal"`
}

type RootFS struct {
	Type   string   `json:"Type"`
	Layers []string `json:"Layers"`
}

type History struct {
	Created    time.Time `json:"created"`
	CreatedBy  string    `json:"created_by"`
	EmptyLayer bool      `json:"empty_layer,omitempty"`
	Comment    string    `json:"comment,omitempty"`
}

type Manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	MediaType     string         `json:"mediaType"`
	Config        ManifestItem   `json:"config"`
	Layers        []ManifestItem `json:"layers"`
}

type ManifestItem struct {
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

type ManifestIndex struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType"`
	Manifests     []ManifestIndexEntry `json:"manifests"`
}

type ManifestIndexEntry struct {
	MediaType string   `json:"mediaType"`
	Size      int64    `json:"size"`
	Digest    string   `json:"digest"`
	Platform  Platform `json:"platform"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Variant      string `json:"variant,omitempty"`
}

type ImageConfigFile struct {
	Architecture string      `json:"architecture"`
	OS           string      `json:"os"`
	Config       ImageConfig `json:"config"`
	RootFS       struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
	History []History `json:"history"`
	Created time.Time `json:"created"`
}

type Reference struct {
	Registry string
	Name     string
	Tag      string
	Digest   string
}
