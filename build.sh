#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$PROJECT_DIR/gui"
BUILD_DIR="$PROJECT_DIR/build"
APP_NAME="PowerfulTop"
APP_BUNDLE="$BUILD_DIR/$APP_NAME.app"

echo "=== Building $APP_NAME ==="
cd "$APP_DIR"

# The root fan helper ships inside the bundle, so it must exist before mygo packages the app.
bash scripts/build-helper.sh
# MIT notices must travel with binary copies, not only the source tree.
cp "$PROJECT_DIR/LICENSE" "$PROJECT_DIR/THIRD_PARTY_NOTICES.md" "$APP_DIR/resources/"
go tool mygo build

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"
# Keep the bundle where the release workflow and the README expect it.
ditto "$APP_DIR/build/darwin-arm64/$APP_NAME.app" "$APP_BUNDLE"
# An ad-hoc signature's designated requirement is the binary's hash, so every build would lose the
# Accessibility grant; naming the bundle identifier keeps it across builds.
BUNDLE_ID="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$APP_BUNDLE/Contents/Info.plist")"
codesign --force --sign - -r="designated => identifier \"$BUNDLE_ID\"" "$APP_BUNDLE"

ZIP_PATH="$BUILD_DIR/$APP_NAME.zip"
ditto -c -k --sequesterRsrc --keepParent "$APP_BUNDLE" "$ZIP_PATH"

echo "=== Build complete ==="
echo "App bundle: $APP_BUNDLE"
echo "Zip archive: $ZIP_PATH"
echo ""
echo "To run: open \"$APP_BUNDLE\""
