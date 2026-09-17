# Installation and Building

Ghost Cache consists of Linux host applications and separate modem firmware.

## Arch Linux

```bash
sudo pacman -S --needed go platformio-core usbutils
```

If PlatformIO is not available from an enabled repository:

```bash
sudo pacman -S --needed python-pipx
pipx install platformio
pipx ensurepath
```

Serial devices on Arch-family systems commonly belong to `uucp`:

```bash
sudo usermod -aG uucp "$USER"
```

Log out and back in before using the new group membership.

## Debian and Raspberry Pi OS

Install Go using the distribution package or the official Go release, then install PlatformIO using `pipx`:

```bash
sudo apt update
sudo apt install -y golang python3-pip pipx usbutils
pipx install platformio
pipx ensurepath
sudo usermod -aG dialout "$USER"
```

Package versions vary. Go 1.24 or newer is required by `go.mod`; PlatformIO Core 6 is recommended. If the distribution Go package is older, install a current official Go toolchain instead.

## Build Host Applications

```bash
go mod download
go test ./...
go build -o ./ghost-node ./cmd/ghost-node
go build -o ./ghost-radio ./cmd/ghost-radio
```

Legacy diagnostic applications can also be built:

```bash
go build -o ./ghost-publish ./cmd/ghost-publish
go build -o ./ghost-broadcast ./cmd/ghost-broadcast
```

The repository build script runs tests, builds every host application, and builds firmware:

```bash
./scripts/build.sh
```

## Build Firmware

```bash
pio run --project-dir firmware/heltec-modem
```

The binary is generated under `firmware/heltec-modem/.pio/build/heltec_v3/`.

## Headless Installation

For a dedicated Linux node, continue with [systemd deployment](service.md). The installer copies the already-built `ghost-node`; it does not compile as root.
