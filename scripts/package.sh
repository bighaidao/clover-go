#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-app}"
ARCH="${ARCH:-$(uname -m)}"
VERSION="${VERSION:-1.1.0}"

# 归一化架构名
case "${ARCH}" in
    arm64|aarch64)
        GO_ARCH="arm64"
        SWIFT_TARGET="arm64-apple-macos12.0"
        ARCH_LABEL="arm64"
        ;;
    x86_64|amd64)
        GO_ARCH="amd64"
        SWIFT_TARGET="x86_64-apple-macos12.0"
        ARCH_LABEL="x86_64"
        ;;
    *)
        echo "❌ 不支持的架构: ${ARCH}"
        exit 1
        ;;
esac

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="${ROOT_DIR}/build"
APP_NAME="CloverGo.app"
APP_DIR="${BUILD_DIR}/${APP_NAME}"
DMG_NAME="CloverGo-${VERSION}-${ARCH_LABEL}.dmg"
DMG_PATH="${BUILD_DIR}/${DMG_NAME}"

echo "=================================================="
echo "  CloverGo (Pure Go + MyGo Metal GPU) 构建打包"
echo "  Version: ${VERSION}"
echo "  Architecture: ${ARCH_LABEL} (GOARCH=${GO_ARCH})"
echo "  Target: ${TARGET}"
echo "=================================================="

# 1. 编译纯 Go 二进制
echo "[1] 编译 Go 二进制 (GOOS=darwin GOARCH=${GO_ARCH} CGO_ENABLED=0)..."
mkdir -p "${BUILD_DIR}"
cd "${ROOT_DIR}"
CGO_ENABLED=0 GOOS=darwin GOARCH="${GO_ARCH}" go build -ldflags="-s -w" -o "${BUILD_DIR}/CloverGo-${ARCH_LABEL}" ./cmd/clover

# 2. 预编译 Swift 原生能力 helper（PDF/水印/Finder sfl3）
echo "[2] 预编译 clover-helper (swiftc -target ${SWIFT_TARGET})..."
swiftc -target "${SWIFT_TARGET}" -O \
    -o "${BUILD_DIR}/clover-helper-${ARCH_LABEL}" \
    "${ROOT_DIR}/tools/clover-helper/main.swift"

# 3. 组装 macOS .app 包结构
echo "[3] 组装 .app 包结构..."
rm -rf "${APP_DIR}"
mkdir -p "${APP_DIR}/Contents/MacOS"
mkdir -p "${APP_DIR}/Contents/Resources"

cp "${BUILD_DIR}/CloverGo-${ARCH_LABEL}" "${APP_DIR}/Contents/MacOS/CloverGo"
chmod +x "${APP_DIR}/Contents/MacOS/CloverGo"
cp "${BUILD_DIR}/clover-helper-${ARCH_LABEL}" "${APP_DIR}/Contents/MacOS/clover-helper"
chmod +x "${APP_DIR}/Contents/MacOS/clover-helper"

# 3.1 组装独立的 Finder 拦截守护进程 Bundle（参考 NewFinder 的 WatchAgent 架构）
echo "[3.1] 组装 CloverGoWatch.app (独立 bundle id 的后台拦截代理)..."
WATCH_APP="${APP_DIR}/Contents/Helpers/CloverGoWatch.app"
mkdir -p "${WATCH_APP}/Contents/MacOS"
cp "${BUILD_DIR}/clover-helper-${ARCH_LABEL}" "${WATCH_APP}/Contents/MacOS/CloverGoWatch"
chmod +x "${WATCH_APP}/Contents/MacOS/CloverGoWatch"

cat <<WEOF > "${WATCH_APP}/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>zh_CN</string>
    <key>CFBundleExecutable</key>
    <string>CloverGoWatch</string>
    <key>CFBundleIdentifier</key>
    <string>com.clover.app.watch</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>CloverGoWatch</string>
    <key>CFBundleDisplayName</key>
    <string>CloverGoWatch</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSPrincipalClass</key>
    <string>NSApplication</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSAppleEventsUsageDescription</key>
    <string>CloverGo 后台监视需要在系统 Finder 被点开时关闭其窗口并拉起 CloverGo。</string>
</dict>
</plist>
WEOF

# 复制应用图标（优先使用 assets/AppIcon.icns 官方高清拟物图标）
if [ -f "${ROOT_DIR}/assets/AppIcon.icns" ]; then
    cp "${ROOT_DIR}/assets/AppIcon.icns" "${APP_DIR}/Contents/Resources/AppIcon.icns"
    cp "${ROOT_DIR}/assets/AppIcon.icns" "${BUILD_DIR}/AppIcon.icns"
elif [ -f "${BUILD_DIR}/AppIcon.icns" ]; then
    cp "${BUILD_DIR}/AppIcon.icns" "${APP_DIR}/Contents/Resources/AppIcon.icns"
fi

cat <<EOF > "${APP_DIR}/Contents/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDisplayName</key>
    <string>CloverGo</string>
    <key>CFBundleExecutable</key>
    <string>CloverGo</string>
    <key>CFBundleIdentifier</key>
    <string>com.clover.app</string>
    <key>CFBundleName</key>
    <string>CloverGo</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleVersion</key>
    <string>100</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSPrincipalClass</key>
    <string>NSApplication</string>
    <key>CFBundleURLTypes</key>
    <array>
        <dict>
            <key>CFBundleURLName</key>
            <string>CloverGo URL</string>
            <key>CFBundleURLSchemes</key>
            <array>
                <string>fclover</string>
            </array>
        </dict>
    </array>
    <key>NSAppleEventsUsageDescription</key>
    <string>CloverGo 需要控制 Finder 来接管打开文件夹请求并自动定位文件。</string>
    <key>NSAppleScriptEnabled</key>
    <true/>
    <key>CFBundleDocumentTypes</key>
    <array>
        <dict>
            <key>CFBundleTypeName</key>
            <string>Folder</string>
            <key>CFBundleTypeOSTypes</key>
            <array>
                <string>fold</string>
            </array>
            <key>CFBundleTypeRole</key>
            <string>Viewer</string>
            <key>LSHandlerRank</key>
            <string>Owner</string>
            <key>LSItemContentTypes</key>
            <array>
                <string>public.folder</string>
                <string>public.directory</string>
                <string>public.volume</string>
            </array>
        </dict>
        <dict>
            <key>CFBundleTypeName</key>
            <string>All Items (Reveal in CloverGo)</string>
            <key>CFBundleTypeRole</key>
            <string>Viewer</string>
            <key>LSHandlerRank</key>
            <string>Owner</string>
            <key>LSItemContentTypes</key>
            <array>
                <string>public.item</string>
                <string>public.data</string>
                <string>public.content</string>
            </array>
        </dict>
    </array>
</dict>
</plist>
EOF

# 4. Ad-Hoc 签名
echo "[4] 执行 macOS xattr 清理与 Ad-Hoc 签名..."
xattr -cr "${APP_DIR}" || true
codesign --force --deep --sign - "${APP_DIR}"
codesign -v "${APP_DIR}"

echo "✅ CloverGo.app 构建完成: ${APP_DIR} (${ARCH_LABEL})"

if [ "${TARGET}" = "dmg" ]; then
    echo "[5] 生成可分发 DMG 镜像: ${DMG_NAME}..."
    rm -f "${DMG_PATH}"
    DMG_STAGING="${BUILD_DIR}/dmg_staging_${ARCH_LABEL}"
    rm -rf "${DMG_STAGING}"
    mkdir -p "${DMG_STAGING}"

    cp -R "${APP_DIR}" "${DMG_STAGING}/"
    ln -s /Applications "${DMG_STAGING}/Applications"

    hdiutil create -volname "CloverGo" -srcfolder "${DMG_STAGING}" -ov -format UDZO "${DMG_PATH}"
    rm -rf "${DMG_STAGING}"
    
    # 生成 SHA256 校验和
    shasum -a 256 "${DMG_PATH}" > "${DMG_PATH}.sha256"
    echo "✅ DMG 镜像生成完成: ${DMG_PATH}"
    echo "🔑 SHA256: $(cat "${DMG_PATH}.sha256")"
fi
