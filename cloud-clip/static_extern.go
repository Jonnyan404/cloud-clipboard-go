//go:build !embed
// +build !embed

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
)

var (
	// 静态文件路径标志
	flg_external_static = flag.String("static", "./static", "外部静态文件目录")

	// 命令行参数
	flg_version      = flag.Bool("v", false, "显示版本信息并退出")
	flg_version_long = flag.Bool("version", false, "显示版本信息并退出")
	flg_config       = flag.String("config", "config.json", "指定配置文件路径")
	flg_host         = flag.String("host", "", "服务器监听地址（覆盖配置文件）")
	flg_port         = flag.Int("port", 9502, "服务器监听端口（覆盖配置文件）")
	flg_prefix       = flag.String("prefix", "", "URL前缀（覆盖配置文件）")
)

// 初始化配置路径全局变量
func init() {
	// 在这里只初始化全局变量，不加载配置
	config_path = *flg_config // 设置默认配置路径
}

// 处理命令行参数的函数
func parseFlags() {
	// 自定义帮助信息
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "云剪贴板 %s\n\n", server_version)
		fmt.Fprintf(os.Stderr, "用法: cloud-clipboard-go [选项]\n\n")
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
	}

	// 解析命令行参数
	flag.Parse()

	// 处理版本查询参数
	if *flg_version || *flg_version_long {
		fmt.Printf("云剪贴板 %s\n", server_version)
		fmt.Printf("Git 提交: %s\n", build_git_hash)
		os.Exit(0)
	}

	// 更新配置文件路径
	config_path = *flg_config
}

// 加载配置并应用命令行覆盖
func loadConfigWithOverrides() *Config {
	// 加载配置文件
	config := load_config(config_path)

	// 使用命令行参数覆盖配置文件中的值
	if *flg_host != "" {
		config.Server.Host = *flg_host
	}
	if *flg_port != 0 {
		config.Server.Port = *flg_port
	}
	if *flg_prefix != "" {
		config.Server.Prefix = *flg_prefix
	}

	return config
}

// 提供静态文件服务
func server_static(prefix string) {
	if *flg_external_static != "" {
		fmt.Println("-- 从外部静态文件目录提供服务:", *flg_external_static)
		// 使用外部静态文件
		http.Handle(prefix+"/", http.StripPrefix(prefix, compressionMiddleware(http.FileServer(http.Dir(*flg_external_static)))))
	}
}
