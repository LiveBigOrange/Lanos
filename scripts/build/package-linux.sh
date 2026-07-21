#!/usr/bin/env bash
# package-linux.sh — Builds Linux packages (AppImage, deb, rpm)
set -euo pipefail

VERSION="${1:-0.1.0}"
OUTPUT_DIR="dist"
APP_NAME="lanos"

echo "Building Lanos Linux packages v$VERSION"

# Build Flutter Linux
cd "$(dirname "$0")/../../ui"
flutter build linux --release

# Build Go core (static)
cd ../core
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o gcd ./cmd/gcd

cd ../scripts/build
mkdir -p "$OUTPUT_DIR"

# --- AppImage ---
echo "Building AppImage..."
APPDIR="$OUTPUT_DIR/appimage/$APP_NAME.AppDir"
rm -rf "$APPDIR"
mkdir -p "$APPDIR/usr/bin"
mkdir -p "$APPDIR/usr/share/applications"
mkdir -p "$APPDIR/usr/share/icons/hicolor/256x256/apps"
mkdir -p "$APPDIR/usr/share/metainfo"

# Copy Flutter app
cp -R ../../ui/build/linux/x64/release/bundle/* "$APPDIR/usr/bin/"
# Copy Go core
cp ../../core/gcd "$APPDIR/usr/bin/"

# Create desktop file
cat > "$APPDIR/usr/share/applications/$APP_NAME.desktop" << EOF
[Desktop Entry]
Name=Lanos
Comment=Local network file sharing
Exec=lanos
Icon=lanos
Type=Application
Categories=Network;FileTransfer;
MimeType=x-scheme-handler/lanos;
EOF

# Create AppRun
cat > "$APPDIR/AppRun" << 'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
export PATH="$HERE/usr/bin:$PATH"
export LD_LIBRARY_PATH="$HERE/usr/lib:$LD_LIBRARY_PATH"
exec "$HERE/usr/bin/lanos" "$@"
EOF
chmod +x "$APPDIR/AppRun"

# Download appimagetool if not present
if ! command -v appimagetool >/dev/null; then
    wget -q https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage -O /tmp/appimagetool
    chmod +x /tmp/appimagetool
    APPIMAGETOOL=/tmp/appimagetool
else
    APPIMAGETOOL=appimagetool
fi

ARCH=x86_64 $APPIMAGETOOL "$APPDIR" "$OUTPUT_DIR/Lanos-$VERSION-x86_64.AppImage"

# --- deb ---
echo "Building deb..."
DEB_DIR="$OUTPUT_DIR/debian"
rm -rf "$DEB_DIR"
mkdir -p "$DEB_DIR/DEBIAN"
mkdir -p "$DEB_DIR/usr/bin"
mkdir -p "$DEB_DIR/usr/share/applications"
mkdir -p "$DEB_DIR/usr/share/icons/hicolor/256x256/apps"
mkdir -p "$DEB_DIR/usr/share/doc/$APP_NAME"

# Copy binaries
cp ../../ui/build/linux/x64/release/bundle/lanos "$DEB_DIR/usr/bin/"
cp ../../core/gcd "$DEB_DIR/usr/bin/"

# Desktop file
cp "$APPDIR/usr/share/applications/$APP_NAME.desktop" "$DEB_DIR/usr/share/applications/"

# Control file
cat > "$DEB_DIR/DEBIAN/control" << EOF
Package: $APP_NAME
Version: $VERSION
Section: net
Priority: optional
Architecture: amd64
Depends: avahi-daemon, libgtk-3-0, libblkid1, liblzma5
Maintainer: Lanos Team <team@lanos.app>
Description: Local network file sharing tool
 Lanos is a zero-config, cross-platform, secure file sharing tool
 for local networks. Share files between Windows, macOS, Linux,
 Android, and iOS devices without internet.
EOF

# Postinst script (firewall setup hint)
cat > "$DEB_DIR/DEBIAN/postinst" << 'EOF'
#!/bin/sh
set -e
if command -v ufw >/dev/null 2>&1; then
    echo "Lanos: To allow LAN connections, run:"
    echo "  sudo /usr/share/lanos/lanos-setup-firewall.sh"
fi
EOF
chmod 755 "$DEB_DIR/DEBIAN/postinst"

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

# Create source tarball
TARBALL="$RPM_DIR/SOURCES/lanos-$VERSION.tar.gz"
tar -czf "$TARBALL" -C "$OUTPUT_DIR" "appimage/$APP_NAME.AppDir"

# Create spec file
cat > "$RPM_DIR/SPECS/lanos.spec" << EOF
Name:           lanos
Version:        $VERSION
Release:        1%{?dist}
Summary:        Local network file sharing tool

License:        MIT
URL:            https://lanos.app
Source0:        %{name}-%{version}.tar.gz

Requires:       avahi, gtk3

%description
Lanos is a zero-config, cross-platform, secure file sharing tool
for local networks.

%prep
%setup -q -c

%install
rm -rf %{buildroot}
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/usr/share/applications
cp -a $APP_NAME.AppDir/usr/bin/* %{buildroot}/usr/bin/
cp -a $APP_NAME.AppDir/usr/share/applications/* %{buildroot}/usr/share/applications/

%files
/usr/bin/lanos
/usr/bin/gcd
/usr/share/applications/lanos.desktop

%changelog
* $(date "+%a %b %d %Y") Lanos Team <team@lanos.app> - $VERSION-1
- Initial release
EOF

rpmbuild --define "_topdir $RPM_DIR" -bb "$RPM_DIR/SPECS/lanos.spec"
cp "$RPM_DIR/RPMS/x86_64/lanos-$VERSION-1."*.rpm "$OUTPUT_DIR/" 2>/dev/null || true

echo "Done. Packages in $OUTPUT_DIR/"
ls -lh "$OUTPUT_DIR/"
