#!/bin/sh
# ipflow 构建脚本
# 用法:
#   ./build.sh              - 构建开发版本 (当前平台)
#   ./build.sh v2.0.0       - 构建指定版本 (当前平台)
#   ./build.sh v2.0.0 all   - 构建指定版本 (所有平台)
#   ./build.sh dev linux    - 构建开发版本 (Linux 平台)

set -e

# ----------------------------------------------------------------------------
# 默认构建设置（可直接在此修改，无需在命令行传参）
# ----------------------------------------------------------------------------
# 版本号：如果想要打特定版本，请修改此处或使用环境变量 VERSION
VERSION="dev"
# 构建类型：current / all / linux / darwin
BUILD_TYPE="current"

# 允许通过命令行参数覆盖默认值（可选）
VERSION=${1:-${VERSION}}
BUILD_TYPE=${2:-${BUILD_TYPE}}

COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo "${RED}[ERROR]${NC} $1"
}

# 清理旧二进制文件
cleanup() {
    if [ -f "ipflow" ]; then
        log_info "Removing old binary..."
        rm -f ipflow
    fi
    # 清理交叉编译产物
    if [ -d "build" ]; then
        log_info "Cleaning build directory..."
        rm -rf build
    fi
}

# 构建函数
build() {
    GOOS=$1
    GOARCH=$2

    OUTPUT_NAME="ipflow"
    if [ "$BUILD_TYPE" = "all" ]; then
        OUTPUT_NAME="build/ipflow-${GOOS}-${GOARCH}"
        mkdir -p build
    fi

    log_info "Building ipflow ${VERSION} for ${GOOS}/${GOARCH}..."

    LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}"

    GOOS=$GOOS GOARCH=$GOARCH go build \
        -ldflags "$LDFLAGS" \
        -o "$OUTPUT_NAME" \
        ./cmd/ipflow

    if [ $? -eq 0 ]; then
        log_info "Build completed: $OUTPUT_NAME"
    else
        log_error "Build failed for ${GOOS}/${GOARCH}"
        return 1
    fi
}

# 主逻辑
case "$BUILD_TYPE" in
    current)
        # 构建当前平台
        cleanup
        log_info "Building ipflow ${VERSION} (${COMMIT}) for current platform..."
        build "$(go env GOOS)" "$(go env GOARCH)"
        log_info "Build completed. Run './ipflow version' to check version."
        ;;
    
    all)
        # 构建所有支持的平台
        cleanup
        log_info "Building ipflow ${VERSION} (${COMMIT}) for all platforms..."
        
        # Linux
        build "linux" "amd64"
        build "linux" "arm64"
        build "linux" "arm"
        
        # macOS
        build "darwin" "amd64"
        build "darwin" "arm64"
        
        # FreeBSD
        build "freebsd" "amd64"
        
        # OpenBSD
        build "openbsd" "amd64"
        
        log_info "All builds completed. Check the 'build' directory for binaries."
        ;;
    
    linux)
        cleanup
        log_info "Building ipflow ${VERSION} for Linux..."
        build "linux" "amd64"
        build "linux" "arm64"
        log_info "Linux builds completed."
        ;;
    
    darwin)
        cleanup
        log_info "Building ipflow ${VERSION} for macOS..."
        build "darwin" "amd64"
        build "darwin" "arm64"
        log_info "macOS builds completed."
        ;;
    
    *)
        log_error "Unknown build type: $BUILD_TYPE"
        echo "Usage: $0 [VERSION] [BUILD_TYPE]"
        echo ""
        echo "BUILD_TYPE options:"
        echo "  current   - Build for current platform (default)"
        echo "  all       - Build for all supported platforms"
        echo "  linux     - Build for Linux (amd64, arm64)"
        echo "  darwin    - Build for macOS (amd64, arm64)"
        exit 1
        ;;
esac
