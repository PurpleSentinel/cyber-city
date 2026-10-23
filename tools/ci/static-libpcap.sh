#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Builds a minimal static libpcap for release binaries (DECISIONS D26) and
# prints the CGO_CFLAGS / CGO_LDFLAGS that make `-lpcap` resolve to it.
#
# Distribution libpcaps differ in soname (Debian libpcap.so.0.8, Fedora
# libpcap.so.1), and Ubuntu's static libpcap pulls in dbus, systemd and RDMA
# libraries. This build has every optional dependency switched off, so the
# release binary needs only glibc.
#
# Usage: tools/ci/static-libpcap.sh <prefix>   (needs gcc, make, flex, bison)
set -euo pipefail
version=1.10.5
# sha256 of libpcap-1.10.5.tar.xz; signature checked against the Tcpdump Group
# release key (1F16 6A57 42AB B9E0 249A  8D30 E089 DEF1 D9C1 5D0D).
sha256=84fa89ac6d303028c1c5b754abff77224f45eca0a94eb1a34ff0aa9ceece3925
prefix=$(realpath -m "${1:?usage: static-libpcap.sh <prefix>}")
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL "https://www.tcpdump.org/release/libpcap-$version.tar.xz" -o "$work/libpcap.tar.xz"
echo "$sha256  $work/libpcap.tar.xz" | sha256sum -c - >&2
tar xJf "$work/libpcap.tar.xz" -C "$work"
cd "$work/libpcap-$version"
./configure --prefix="$prefix" --disable-shared --enable-static \
  --disable-dbus --disable-rdma --disable-bluetooth --disable-usb \
  --disable-netmap --disable-dpdk --without-libnl --without-dag \
  --without-septel --without-snf --without-turbocap >&2
make -j"$(nproc)" >&2
make install >&2

echo "CGO_CFLAGS=-I$prefix/include"
echo "CGO_LDFLAGS=-L$prefix/lib"
