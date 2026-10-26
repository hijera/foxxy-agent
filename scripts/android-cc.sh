#!/usr/bin/env bash
# Print the NDK clang that links the Android build of FoxxyCode for one GOARCH.
#
# The Android build is linked with cgo against Bionic (see
# docs/getting-started/android.md), so it needs the Android NDK. The NDK is
# taken from ANDROID_NDK_HOME, ANDROID_NDK_ROOT or ANDROID_NDK - the GitHub
# runners set all three - and otherwise from the newest ndk/<version> under
# ANDROID_HOME, ANDROID_SDK_ROOT or the Android Studio default SDK. API 24
# (Android 7.0), the oldest Android Termux runs on, is the target unless
# ANDROID_API says otherwise.
#
# Usage:
#   scripts/android-cc.sh arm64|amd64
set -euo pipefail

api="${ANDROID_API:-24}"

case "${1:-}" in
    arm64) triple=aarch64-linux-android ;;
    amd64) triple=x86_64-linux-android ;;
    *) echo "usage: $0 arm64|amd64" >&2; exit 2 ;;
esac

ndk=""
for candidate in "${ANDROID_NDK_HOME:-}" "${ANDROID_NDK_ROOT:-}" "${ANDROID_NDK:-}"; do
    if [ -n "$candidate" ] && [ -d "$candidate/toolchains/llvm/prebuilt" ]; then
        ndk="$candidate"
        break
    fi
done
if [ -z "$ndk" ]; then
    for sdk in "${ANDROID_HOME:-}" "${ANDROID_SDK_ROOT:-}" "$HOME/Android/Sdk" "$HOME/Library/Android/sdk"; do
        if [ -z "$sdk" ] || [ ! -d "$sdk/ndk" ]; then
            continue
        fi
        latest=$(ls -1 "$sdk/ndk" | sort -V | tail -n 1)
        if [ -n "$latest" ] && [ -d "$sdk/ndk/$latest/toolchains/llvm/prebuilt" ]; then
            ndk="$sdk/ndk/$latest"
            break
        fi
    done
fi
if [ -z "$ndk" ]; then
    echo "no Android NDK found: set ANDROID_NDK_HOME, or install one with" >&2
    echo "  sdkmanager \"ndk;27.3.13750724\"" >&2
    exit 1
fi

for prebuilt in "$ndk"/toolchains/llvm/prebuilt/*/; do
    cc="${prebuilt%/}/bin/${triple}${api}-clang"
    if [ -x "$cc" ]; then
        echo "$cc"
        exit 0
    fi
done
echo "no ${triple}${api}-clang in the NDK at $ndk" >&2
exit 1
