#!/bin/bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$PROJECT_DIR/build/tests"

if [ -d "/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk" ]; then
    SDK="/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk"
    SWIFTC="/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/swiftc"
else
    SDK="$(xcrun --show-sdk-path)"
    SWIFTC="$(xcrun --find swiftc)"
fi

mkdir -p "$BUILD_DIR"

"$SWIFTC" \
    -target arm64-apple-macosx14.0 \
    -sdk "$SDK" \
    -framework Foundation \
    -framework CoreFoundation \
    -framework IOKit \
    -parse-as-library \
    -Onone \
    -o "$BUILD_DIR/PowerfulTopRegressionTests" \
    "$PROJECT_DIR"/PowerfulTop/Models/*.swift \
    "$PROJECT_DIR"/PowerfulTop/Utilities/*.swift \
    "$PROJECT_DIR"/Tests/*.swift

"$BUILD_DIR/PowerfulTopRegressionTests"

python3 -I "$PROJECT_DIR/scripts/check-localization.py" "$PROJECT_DIR"
