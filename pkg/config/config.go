package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	RootDir        string
	ContainersDir  string
	ImagesDir      string
	VolumesDir     string
	NetworksDir    string
	LogsDir        string
	SocketPath     string
	DefaultRuntime string
}

var Global *Config

func Init() {
	root := dataDir()
	Global = &Config{
		RootDir:        root,
		ContainersDir:  filepath.Join(root, "containers"),
		ImagesDir:      filepath.Join(root, "images"),
		VolumesDir:     filepath.Join(root, "volumes"),
		NetworksDir:    filepath.Join(root, "networks"),
		LogsDir:        filepath.Join(root, "logs"),
		SocketPath:     filepath.Join(root, "cryruss.sock"),
		DefaultRuntime: "runc",
	}
	for _, d := range []string{
		Global.ContainersDir,
		Global.ImagesDir,
		Global.VolumesDir,
		Global.NetworksDir,
		Global.LogsDir,
	} {
		os.MkdirAll(d, 0755)
	}
}

func dataDir() string {
	if v := os.Getenv("CRYRUSS_DATA"); v != "" {
		return v
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "cryruss")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "cryruss")
}
