# cryruss 

cryruss is version minimal of docker maybe non-root =)

## Build

```
make build
# binary at dist/cryruss
```

## Usage

```
cryruss run alpine echo hello
cryruss run -d -p 8080:80 nginx
cryruss ps -a
cryruss logs <id>
cryruss exec -it <id> sh
cryruss images
cryruss pull ubuntu:22.04
cryruss network ls
cryruss volume create mydata
cryruss serve   # start REST API on unix socket
```

## API

Compatible with Docker API v1.41 subset.
Socket: `~/.local/share/cryruss/cryruss.sock`

Override data directory: `CRYRUSS_DATA=/path cryruss run ...`

## Requirements

- Linux kernel 3.8+ (user namespaces)
- `/proc/self/exe` access
- `nsenter` (for `exec` subcommand)
- `fuse-overlayfs` (optional, for overlay rootfs)
