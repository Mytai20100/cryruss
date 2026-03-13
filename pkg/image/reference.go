package image

import "strings"

func ParseReference(ref string) Reference {
	r := Reference{Tag: "latest"}

	// Handle digest
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		r.Digest = ref[idx+1:]
		ref = ref[:idx]
		r.Tag = ""
	}

	// Handle tag
	if idx := strings.LastIndex(ref, ":"); idx > 0 {
		// Make sure it's not a registry:port situation
		rest := ref[idx+1:]
		if !strings.Contains(rest, "/") && r.Digest == "" {
			r.Tag = rest
			ref = ref[:idx]
		}
	}

	// Parse registry and name
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 1 {
		r.Registry = "registry-1.docker.io"
		r.Name = "library/" + parts[0]
	} else {
		// Check if first part looks like a registry (contains dot or colon or is localhost)
		if strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost" {
			r.Registry = parts[0]
			r.Name = parts[1]
		} else {
			r.Registry = "registry-1.docker.io"
			r.Name = ref
		}
	}

	return r
}

func (r Reference) String() string {
	name := r.Name
	// Strip library/ prefix for display
	if strings.HasPrefix(name, "library/") {
		name = strings.TrimPrefix(name, "library/")
	}

	s := name
	if r.Registry != "registry-1.docker.io" {
		s = r.Registry + "/" + name
	}
	if r.Digest != "" {
		return s + "@" + r.Digest
	}
	return s + ":" + r.Tag
}

func (r Reference) AuthURL() string {
	if r.Registry == "registry-1.docker.io" {
		return "https://auth.docker.io"
	}
	return "https://" + r.Registry
}

func (r Reference) RegistryURL() string {
	if r.Registry == "registry-1.docker.io" {
		return "https://registry-1.docker.io"
	}
	return "https://" + r.Registry
}

func (r Reference) TagOrDigest() string {
	if r.Digest != "" {
		return r.Digest
	}
	return r.Tag
}
