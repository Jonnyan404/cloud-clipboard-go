package main

import (
	"flag"
	"fmt"
	"os"
)

// 定义所有命令行参数
var (
	// flg_external_static = flag.String("static", "./static", "Path to external static files")
	flg_config  = flag.String("config", "config.json", "指定配置文件路径")
	flg_version = flag.Bool("v", false, "显示版本信息并退出")
)

func init() {
	// 解析命令行参数
	flag.Parse()

	// 如果指定了版本参数，显示版本并退出
	if *flg_version {
		fmt.Printf("Cloud Clipboard %s\n", server_version)
		os.Exit(0)
	}
}
