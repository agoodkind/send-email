#!/usr/bin/env bash
set -euo pipefail

PACKAGING_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

build_deb() {
    local tag="$1"
    local arch="$2"
    local output_dir="$3"
    local work_dir
    work_dir="$(mktemp -d)"
    local root="$work_dir/root"

    gh release download "$tag" --repo agoodkind/send-email \
        --pattern "send-email_linux_${arch}.tar.gz" --dir "$work_dir"
    install -d "$root/DEBIAN" "$root/usr/bin" "$root/usr/sbin"
    tar -xzf "$work_dir/send-email_linux_${arch}.tar.gz" -C "$root/usr/bin" send-email
    ln -s ../bin/send-email "$root/usr/sbin/sendmail"
    sed -e "s/@VERSION@/${tag}/" -e "s/@ARCH@/${arch}/" \
        "$PACKAGING_DIR/control" >"$root/DEBIAN/control"
    dpkg-deb --root-owner-group --build "$root" \
        "$output_dir/send-email_${tag}_${arch}.deb"
    rm -rf "$work_dir"
}

main() {
    local tag="${1:?Supply the release tag}"
    local output_dir="${2:?Supply the output directory}"
    local arch

    install -d "$output_dir"
    for arch in amd64 arm64; do
        build_deb "$tag" "$arch" "$output_dir"
    done
}

main "$@"
