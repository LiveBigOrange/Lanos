#!/usr/bin/env bash
# package-macos.sh — Builds macOS .dmg installer
set -euo pipefail

VERSION="${1:-0.1.0}"
OUTPUT_DIR="dist"
APP_NAME="Lanos"
BUNDLE_ID="com.lanos.lanos"

SKIP_BUILD="${2:-}"
echo "Building Lanos macOS installer v$VERSION"

if [ -z "$SKIP_BUILD" ]; then
    cd "$(dirname "$0")/../../ui"
    flutter build macos --release
    cd ../core
    GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o gcd-amd64 ./cmd/gcd
    GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o gcd-arm64 ./cmd/gcd
    lipo -create -output gcd gcd-amd64 gcd-arm64
    rm gcd-amd64 gcd-arm64
fi

# Create output directory
cd ../scripts/build
mkdir -p "$OUTPUT_DIR"

# Create app bundle structure
APP_BUNDLE="$OUTPUT_DIR/$APP_NAME.app"
rm -rf "$APP_BUNDLE"
mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Resources"
mkdir -p "$APP_BUNDLE/Contents/Frameworks"

# Copy Flutter app
cp -R "../../ui/build/macos/Build/Products/Release/$APP_NAME.app/"* "$APP_BUNDLE/"

# Copy Go core
cp "../../core/gcd" "$APP_BUNDLE/Contents/MacOS/"

# Create Info.plist
cat > "$APP_BUNDLE/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleExecutable</key>
    <string>lanos</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>$BUNDLE_ID</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>$APP_NAME</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSLocalNetworkUsageDescription</key>
    <string>Lanos needs local network access to discover and share files with nearby devices.</string>
    <key>NSBonjourServices</key>
    <array>
        <string>_lanos._tcp</string>
    </array>
</dict>
</plist>
EOF

# Create DMG
DMG_PATH="$OUTPUT_DIR/Lanos-$VERSION.dmg"
rm -f "$DMG_PATH"
hdiutil create -volname "$APP_NAME" -srcfolder "$APP_BUNDLE" -ov -format UDZO "$DMG_PATH"

echo "Done. DMG: $DMG_PATH"
