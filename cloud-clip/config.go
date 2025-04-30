package main

/**
*** FILE: config.go
***   handle config.json <===> Config
**/

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	// "github.com/sanity-io/litter"
)

type Config struct {
	Server struct {
		Host        interface{} `json:"host"`        //done
		Port        int         `json:"port"`        //done
		Prefix      string      `json:"prefix"`      //done
		History     int         `json:"history"`     //done
		HistoryFile string      `json:"historyFile"` // 添加历史文件路径
		StorageDir  string      `json:"storageDir"`  // 添加存储目录路径
		// Auth    string `json:"auth"`
		Auth interface{} `json:"auth"` //done
		SSL  struct {
			Enabled bool   `json:"enabled"`
			Cert    string `json:"cert"`
			Key     string `json:"key"`
		} `json:"ssl"`
	} `json:"server"`
	Text struct {
		Limit int `json:"limit"` //done
	} `json:"text"`
	File struct {
		Expire int `json:"expire"` //done
		Chunk  int `json:"chunk"`  //done, but no limit
		Limit  int `json:"limit"`  //done
	} `json:"file"`
}

var config_path = "config.json"

func load_config(config_path string) *Config {
	_ = build_git_hash // improve global var init order

	// Read the config.json file_content
	// file_content, err := ioutil.ReadFile("config.json")
	file_content, err := os.ReadFile(config_path)
	need_create := false
	if err != nil {
		// fmt.Printf("Error reading config file: %v\n", err)
		// os.Exit(1)
		file_content = []byte("{}")
		need_create = true
	}

	// Parse the JSON data into the Config struct
	var config Config
	err = json.Unmarshal(file_content, &config)
	if err != nil {
		fmt.Printf("Error parsing config file: %v\n", err)
		os.Exit(1)
	}

	// Set default values
	if config.Server.Port == 0 {
		config.Server.Port = 9501
	}

	config.Server.Prefix = strings.TrimRight(config.Server.Prefix, "/")

	if config.Server.History == 0 {
		config.Server.History = 100
	}
	if config.Server.Auth != nil {
		switch auth := config.Server.Auth.(type) {
		case bool:
			fmt.Printf("Auth is bool: %v\n", auth)
		case string:
			fmt.Printf("Auth is str: %s\n", auth)
		default:
			fmt.Printf("Auth is unexpected type: %T\n", auth)
		}
	} else {
		// fmt.Println("Auth field is not provided in the config file")
		config.Server.Auth = false
	}

	if config.Server.Host != nil {
		switch host := config.Server.Host.(type) {
		case string:
			// 如果是字符串，转换为单元素数组
			fmt.Printf("Host is string: %s\n", host)
			config.Server.Host = []string{host}
		case []interface{}:
			// 如果是接口数组，转换为字符串数组
			hostList := make([]string, 0, len(host))
			for i, h := range host {
				if hostStr, ok := h.(string); ok {
					hostList = append(hostList, hostStr)
					fmt.Printf("Host[%d]: %s\n", i, hostStr)
				} else {
					fmt.Printf("Warning: Host[%d] is not a string: %T\n", i, h)
					// 跳过无效元素
				}
			}
			if len(hostList) == 0 {
				hostList = []string{"0.0.0.0"}
				fmt.Printf("No valid hosts in array, using default: 0.0.0.0\n")
			}
			config.Server.Host = hostList
		default:
			fmt.Printf("Host is unexpected type: %T, using default\n", host)
			config.Server.Host = []string{"0.0.0.0"}
		}
	} else {
		// 如果未提供，设置默认值
		config.Server.Host = []string{"0.0.0.0"}
	}

	if config.Text.Limit == 0 {
		config.Text.Limit = 4096
	}
	if config.File.Expire == 0 {
		config.File.Expire = 3600
	}
	if config.File.Chunk == 0 {
		config.File.Chunk = 2 * _MB
	}
	if config.File.Limit == 0 {
		config.File.Limit = 256 * _MB
	}

	if need_create {
		// fmt.Println("create default config:")
		// config_json, err := json.Marshal(config)
		config_json, err := json.MarshalIndent(config, "", "\t")
		if err == nil {
			err = os.WriteFile(config_path, config_json, 0644)
			if err != nil {
				fmt.Printf("Error writing config file: %v\n", err)
				// os.Exit(1)
			}
			// log.Println("++ config.json created with default config")
		}
	}

	if config.Server.Auth == false { // convert to ""
		config.Server.Auth = ""
	}

	// Print the parsed configuration
	// fmt.Printf("\n---Parsed Config: %+v\n", config)
	// fmt.Println("\n---litter.dump:")
	// litter.Dump(config)

	return &config
}

/*

func typeof2(v interface{}) string {
	return reflect.TypeOf(v).String()
}

func main() {
	conf := load_config(config_path)

	fmt.Println("auth:", conf.Server.Auth, conf.Server.Auth == false)
	// fmt.Println("auth:", fmt.Sprintf("%T", conf.Server.Auth))
	fmt.Println("auth:", typeof(conf.Server.Auth))

}

//*/
