#!/usr/bin/env bash
# Install the native SDK runner dependencies on Ubuntu CI workers.
set -euo pipefail

# Use the canonical Ubuntu package host for both mirrorlist-based runner
# images and images with direct Ubuntu sources. Keep APT signature checking.
for source_file in \
  /etc/apt/apt-mirrors.txt \
  /etc/apt/apt-security-mirrors.txt \
  /etc/apt/sources.list \
  /etc/apt/sources.list.d/ubuntu.sources; do
  if [[ -f "$source_file" ]]; then
    sudo sed -i 's|azure\.archive\.ubuntu\.com|archive.ubuntu.com|g' "$source_file"
  fi
done

sudo apt-get update
sudo apt-get install --no-install-recommends -y \
  gdb clang cmake ninja-build pkg-config libgtk-3-dev libayatana-appindicator3-dev \
  protobuf-compiler libprotobuf-dev libpulse-dev pulseaudio pulseaudio-utils xvfb xauth
