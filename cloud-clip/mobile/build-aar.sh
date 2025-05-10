#!/bin/sh

# 设置环境变量
export ANDROID_HOME=$HOME/Library/Android/sdk
export ANDROID_NDK_HOME=$HOME/Library/Android/sdk/ndk/29.0.13113456
export PATH=$PATH:$HOME/go/bin
export GOPATH=$HOME/go
export GO111MODULE=on

echo "Setting up dependencies..."
# 确保所需的依赖项存在
go get -u golang.org/x/mobile/bind

echo "Using NDK: $ANDROID_NDK_HOME"
echo "Binding package..."

gomobile bind -tags embed -androidapi 24 -o cloudclipservice.aar -target=android \
  -ldflags="-s -w -X lib.ServerVersion=android-1.0.0 -X lib.UseEmbeddedStr=true"

if [ $? -eq 0 ]; then
  echo "Build successful. AAR file generated: cloud-clip/mobile/cloudclipservice.aar"
  mkdir -p ../../Android/app/libs
  cp cloudclipservice.aar ../../Android/app/libs/
  echo "Copied AAR file to Android project libs folder"
else
  echo "Build failed!"
fi