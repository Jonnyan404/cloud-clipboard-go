#!/bin/bash
echo "Building for macOS..."
cargo tauri build --target x86_64-apple-darwin
echo "Build complete! Check src-tauri/target/release/bundle/dmg/ for .dmg file."