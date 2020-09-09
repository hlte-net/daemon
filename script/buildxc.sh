#!/bin/bash
# adapted from from https://www.digitalocean.com/community/tutorials/how-to-build-go-executables-for-multiple-platforms-on-ubuntu-16-04

platforms=("windows/amd64" "windows/386" "darwin/amd64" "linux/amd64" "linux/386" "linux/arm" "linux/arm64")

PKG_NAME="hlte-daemon"
OG_OS=$GOOS
OG_ARCH=$GOARCH

mkdir -p build

for platform in "${platforms[@]}"
do
    platform_split=(${platform//\// })
    export GOOS=${platform_split[0]}
    export GOARCH=${platform_split[1]}

    output_name=$PKG_NAME'.'$GOOS'-'$GOARCH
    if [ "$GOOS" = "windows" ]; then
        output_name+='.exe'
    fi

    echo -n "${platform}... "
    go build -o "build/${output_name}" hlte.net/daemon
    echo -e "✓"
done

export GOOS=$OG_OS
export GOARCH=$OG_ARCH

pushd build
ls -1 ./ | xargs -I{} shasum -a 256 {}
popd
