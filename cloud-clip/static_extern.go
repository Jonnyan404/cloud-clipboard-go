//go:build !embed
// +build !embed

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

// specify external static folder
var flg_external_static = flag.String("static", "./static", "Path to external static files")

// 添加版本和配置文件参数
var flg_version = flag.Bool("v", false, "显示版本信息并退出")
var flg_config = flag.String("config", "config.json", "指定配置文件路径")

// 在初始化函数中处理版本参数
func init() {
	// 注意：这会在 main() 之前执行
	flag.Parse()

	// 如果指定了版本参数，显示版本并退出
	if *flg_version {
		fmt.Printf("Cloud Clipboard %s\n", server_version)
		os.Exit(0)
	}
}

func server_static(prefix string) {
	if *flg_external_static != "" {
		fmt.Println("-- serve from external static:", *flg_external_static)
		// use external static
		http.Handle(prefix+"/", http.StripPrefix(prefix, compressionMiddleware(http.FileServer(http.Dir(*flg_external_static)))))
	}
}
