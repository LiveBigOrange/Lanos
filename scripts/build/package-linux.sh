#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-0.1.0}"
VERSION="${VERSION#v}"
OUTPUT_DIR="dist"
APP_NAME="lanos"

SKIP_BUILD="${2:-}"
echo "Packaging Lanos Linux v$VERSION"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

if [ -z "$SKIP_BUILD" ]; then
    cd "$PROJECT_ROOT/core"
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o gcd ./cmd/gcd
fi

cd "$SCRIPT_DIR"
mkdir -p "$OUTPUT_DIR"

GCD_BIN="$PROJECT_ROOT/core/gcd"

# --- deb ---
echo "Building deb..."
DEB_DIR="$OUTPUT_DIR/debian"
rm -rf "$DEB_DIR"
mkdir -p "$DEB_DIR/DEBIAN"
mkdir -p "$DEB_DIR/usr/bin"
mkdir -p "$DEB_DIR/usr/share/applications"
mkdir -p "$DEB_DIR/usr/share/doc/$APP_NAME"

cp "$GCD_BIN" "$DEB_DIR/usr/bin/"

cat > "$DEB_DIR/usr/share/applications/$APP_NAME.desktop" << EOF
[Desktop Entry]
Name=Lanos
Comment=Secure P2P file transfer over LAN
Exec=gcd
Type=Application
Categories=Network;FileTransfer;
Terminal=false
EOF

cat > "$DEB_DIR/DEBIAN/control" << EOF
Package: $APP_NAME
Version: $VERSION
Section: net
Priority: optional
Architecture: amd64
Depends: avahi-daemon
Maintainer: Lanos Team <team@lanos.app>
Description: Secure P2P file transfer over LAN
 Lanos is a zero-config, secure file sharing tool for local networks.
 It uses Noise Protocol encryption and mDNS discovery.
EOF

dpkg-deb --build "$DEB_DIR" "$OUTPUT_DIR/lanos_${VERSION}_amd64.deb"

# --- rpm ---
echo "Building rpm..."
RPM_DIR="$OUTPUT_DIR/rpm"
rm -rf "$RPM_DIR"
mkdir -p "$RPM_DIR/BUILD"
mkdir -p "$RPM_DIR/RPMS"
mkdir -p "$RPM_DIR/SOURCES"
mkdir -p "$RPM_DIR/SPECS"
mkdir -p "$RPM_DIR/SRPMS"

RPM_TOPDIR="$(cd "$RPM_DIR" && pwd)"

mkdir -p "$RPM_DIR/BUILD/lanos-$VERSION/usr/bin"
mkdir -p "$RPM_DIR/BUILD/lanos-$VERSION/usr/share/applications"
cp "$GCD_BIN" "$RPM_DIR/BUILD/lanos-$VERSION/usr/bin/"
cp "$DEB_DIR/usr/share/applications/$APP_NAME.desktop" "$RPM_DIR/BUILD/lanos-$VERSION/usr/share/applications/"

tar -czf "$RPM_DIR/SOURCES/lanos-$VERSION.tar.gz" -C "$RPM_DIR/BUILD" "lanos-$VERSION"

cat > "$RPM_DIR/SPECS/lanos.spec" << EOF
Name:           lanos
Version:        $VERSION
Release:        1%{?dist}
Summary:        Secure P2P file transfer over LAN

License:        MIT
URL:            https://github.com/LiveBigOrange/Lanos
Source0:        %{name}-%{version}.tar.gz

Requires:       avahi

%description
Lanos is a zero-config, secure file sharing tool for local networks.

%prep
%setup -q

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/usr/share/applications
cp -a usr/bin/* %{buildroot}/usr/bin/
cp -a usr/share/applications/* %{buildroot}/usr/share/applications/

%files
/usr/bin/gcd
/usr/share/applications/lanos.desktop

%changelog
* $(date "+%a %b %d %Y") Lanos Team <team@lanos.app> - $VERSION-1
- Initial release
EOF

rpmbuild --define "_topdir $RPM_TOPDIR" -bb "$RPM_DIR/SPECS/lanos.spec"
cp "$RPM_DIR/RPMS/x86_64/lanos-$VERSION-1."*.rpm "$OUTPUT_DIR/" 2>/dev/null || true

echo "Done. Packages in $OUTPUT_DIR/"
ls -lh "$OUTPUT_DIR/"
