package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// 定义所有命令行参数
var (
	// flg_external_static = flag.String("static", "./static", "Path to external static files")
	flg_config       = flag.String("config", "config.json", "指定配置文件路径")
	flg_version      = flag.Bool("v", false, "显示版本信息并退出")
	flg_host         = flag.String("host", "", "指定监听主机地址，如果设置则覆盖配置文件")
	flg_port         = flag.Int("port", 0, "指定监听端口，如果设置则覆盖配置文件")
	flg_auth         = flag.String("auth", "", "指定访问密码,如果设置则覆盖配置文件")
	flg_storage_dir  = flag.String("storage", "", "指定文件存储目录，如果设置则覆盖配置文件")
	flg_history_file = flag.String("historyfile", "", "指定历史记录文件路径，如果设置则覆盖配置文件")
	flg_help         = flag.Bool("h", false, "显示帮助信息")
)

// 自定义帮助信息，格式更美观
func printHelp() {
	appName := os.Args[0]
	fmt.Printf("Cloud Clipboard %s\n\n", server_version)
	fmt.Printf("用法: %s [选项]\n\n", appName)
	fmt.Println("选项:")
	flag.PrintDefaults()
	fmt.Println("\n示例:")
	fmt.Printf("  %s -port 9502                  # 在端口9502上启动服务\n", appName)
	fmt.Printf("  %s -host 127.0.0.1 -port 9502  # 在127.0.0.1:9502上启动服务\n", appName)
	fmt.Printf("  %s -config myconfig.json       # 使用指定的配置文件\n", appName)
	fmt.Printf("  %s -auth abcdefg      		 # 使用指定的字符串作为网站访问密码\n", appName)

}

func init() {
	// 自定义帮助信息
	flag.Usage = printHelp

	// 解析命令行参数
	flag.Parse()

	// 检查是否有未知参数
	if flag.NArg() > 0 {
		unknownArgs := strings.Join(flag.Args(), ", ")
		fmt.Printf("错误: 未知参数: %s\n\n", unknownArgs)
		printHelp()
		os.Exit(1)
	}

	// 检查参数是否有效
	validArgs := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		validArgs[f.Name] = true
	})

	// 检查参数是否有值
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-") {
			argName := strings.TrimLeft(arg, "-")
			if strings.Contains(argName, "=") {
				argName = strings.Split(argName, "=")[0]
			}
			if !validArgs[argName] && argName != "h" && argName != "v" {
				fmt.Printf("错误: 未知参数: -%s\n\n", argName)
				printHelp()
				os.Exit(1)
			}
		}
	}

	// 如果指定了帮助参数，显示帮助并退出
	if *flg_help {
		printHelp()
		os.Exit(0)
	}

	// 如果指定了版本参数，显示版本并退出
	if *flg_version {
		fmt.Printf("Cloud Clipboard %s\n", server_version)
		os.Exit(0)
	}
}

// 应用命令行参数，覆盖配置文件中的设置
func applyCommandLineArgs() {
	// 如果命令行指定了主机地址，覆盖配置
	if *flg_host != "" {
		fmt.Printf("使用命令行指定的主机地址: %s\n", *flg_host)
		config.Server.Host = *flg_host
	}

	// 如果命令行指定了端口，覆盖配置
	if *flg_port > 0 {
		fmt.Printf("使用命令行指定的端口: %d\n", *flg_port)
		config.Server.Port = *flg_port
	}
	if *flg_auth != "" {
		fmt.Printf("使用命令行指定的访问密码: %s\n", *flg_auth)
		config.Server.Auth = *flg_auth
	}
	if *flg_storage_dir != "" {
		fmt.Printf("使用命令行指定的存储目录: %s\n", *flg_storage_dir)
		config.Server.StorageDir = *flg_storage_dir
	}
	if *flg_history_file != "" {
		fmt.Printf("使用命令行指定的历史文件: %s\n", *flg_history_file)
		config.Server.HistoryFile = *flg_history_file
	}
}
