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
    -o "$BUILD_DIR/PowerTopRegressionTests" \
    "$PROJECT_DIR/PowerTop/Utilities/IOKitHelpers.swift" \
    "$PROJECT_DIR/PowerTop/Utilities/BatteryTimeEstimator.swift" \
    "$PROJECT_DIR/PowerTop/Utilities/PowerFlow.swift" \
    "$PROJECT_DIR/PowerTop/Utilities/BatteryChargeLimit.swift" \
    "$PROJECT_DIR/PowerTop/Models/PowerConnectionPhase.swift" \
    "$PROJECT_DIR/PowerTop/Models/PowerData.swift" \
    "$PROJECT_DIR/Tests/RegressionTests.swift"

"$BUILD_DIR/PowerTopRegressionTests"
