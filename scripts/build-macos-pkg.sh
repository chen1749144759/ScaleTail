#!/usr/bin/env bash
set -euo pipefail

version="${VERSION:-0.0.1}"
goarch=""
out_dir="dist/macos"

while [[ $# -gt 0 ]]; do
  case "$1" in
  --version)
    version="$2"
    shift 2
    ;;
  --arch)
    goarch="$2"
    shift 2
    ;;
  --out)
    out_dir="$2"
    shift 2
    ;;
  *)
    echo "unknown argument: $1" >&2
    exit 2
    ;;
  esac
done

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "build-macos-pkg.sh must run on macOS because it uses pkgbuild." >&2
  exit 1
fi

if [[ -z "$goarch" ]]; then
  case "$(uname -m)" in
  x86_64)
    goarch="amd64"
    ;;
  arm64)
    goarch="arm64"
    ;;
  *)
    echo "unsupported host architecture: $(uname -m)" >&2
    exit 1
    ;;
  esac
fi

case "$goarch" in
amd64 | arm64)
  ;;
*)
  echo "unsupported macOS architecture: $goarch" >&2
  exit 1
  ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

if ! command -v pkgbuild >/dev/null 2>&1; then
  echo "pkgbuild not found. Install Xcode Command Line Tools on macOS." >&2
  exit 1
fi

go_tool="./tool/go"
if [[ ! -x "$go_tool" ]]; then
  go_tool="go"
fi

pkg_id="com.scaletail.scaletail"
launchd_label="com.scaletail.scaletaild"
work_dir="${TMPDIR:-/tmp}/scaletail-macos-${version}-${goarch}"
pkg_root="$work_dir/root"
scripts_dir="$work_dir/scripts"
pkg_out="$out_dir/ScaleTail-${version}-darwin-${goarch}.pkg"
tar_out="$out_dir/scaletail_${version}_darwin_${goarch}.tar.gz"
sha_out="$out_dir/SHA256SUMS-macos-${goarch}.txt"

rm -rf "$work_dir"
mkdir -p \
  "$pkg_root/usr/local/bin" \
  "$pkg_root/Library/LaunchDaemons" \
  "$pkg_root/Library/ScaleTail" \
  "$scripts_dir" \
  "$out_dir"

echo "Building scaletail for darwin/$goarch"
TS_VERSION_OVERRIDE="$version" GOOS=darwin GOARCH="$goarch" "$go_tool" build -trimpath -o "$pkg_root/usr/local/bin/scaletail" ./cmd/scaletail

echo "Building scaletaild for darwin/$goarch"
TS_VERSION_OVERRIDE="$version" GOOS=darwin GOARCH="$goarch" "$go_tool" build -trimpath -o "$pkg_root/usr/local/bin/scaletaild" ./cmd/scaletaild

chmod 0755 "$pkg_root/usr/local/bin/scaletail" "$pkg_root/usr/local/bin/scaletaild"
chmod 0700 "$pkg_root/Library/ScaleTail"

cat >"$pkg_root/Library/LaunchDaemons/${launchd_label}.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${launchd_label}</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/scaletaild</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
PLIST

cat >"$scripts_dir/preinstall" <<'SCRIPT'
#!/bin/sh
set -e

label="com.scaletail.scaletaild"
plist="/Library/LaunchDaemons/${label}.plist"

if launchctl print "system/${label}" >/dev/null 2>&1; then
  launchctl bootout "system/${label}" >/dev/null 2>&1 || true
fi

launchctl unload "$plist" >/dev/null 2>&1 || true
exit 0
SCRIPT

cat >"$scripts_dir/postinstall" <<'SCRIPT'
#!/bin/sh
set -e

label="com.scaletail.scaletaild"
plist="/Library/LaunchDaemons/${label}.plist"

mkdir -p /Library/ScaleTail
chown root:wheel /Library/ScaleTail
chmod 700 /Library/ScaleTail

chown root:wheel /usr/local/bin/scaletail /usr/local/bin/scaletaild "$plist"
chmod 755 /usr/local/bin/scaletail /usr/local/bin/scaletaild
chmod 644 "$plist"

if ! launchctl bootstrap system "$plist" >/dev/null 2>&1; then
  launchctl load "$plist" >/dev/null 2>&1 || true
fi

launchctl kickstart -k "system/${label}" >/dev/null 2>&1 || launchctl start "$label" >/dev/null 2>&1 || true
exit 0
SCRIPT

chmod 0755 "$scripts_dir/preinstall" "$scripts_dir/postinstall"

echo "Building unsigned package $pkg_out"
pkgbuild \
  --root "$pkg_root" \
  --scripts "$scripts_dir" \
  --identifier "$pkg_id" \
  --version "$version" \
  --install-location / \
  --ownership recommended \
  "$pkg_out"

echo "Building tarball $tar_out"
tar -C "$pkg_root" -czf "$tar_out" .

(
  cd "$out_dir"
  shasum -a 256 "$(basename "$pkg_out")" "$(basename "$tar_out")" >"$(basename "$sha_out")"
)

echo "Built:"
ls -lh "$pkg_out" "$tar_out" "$sha_out"
