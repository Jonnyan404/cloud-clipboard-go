/**
*** FILE: auth.go
***   处理认证和内容直接访问功能
**/

package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
)

// 获取客户端真实IP地址
func getAuthClientIP(r *http.Request) string {
	// 尝试从X-Real-IP头获取
	ip := r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}

	// 尝试从X-Forwarded-For头获取
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// 使用已有的 get_remote 函数获取
	clientIP, _ := get_remote(r)
	return clientIP
}

// 验证请求的认证信息
func validateAuth(r *http.Request) bool {
	// 如果配置中未启用认证，直接返回成功
	if config.Server.Auth == "" || config.Server.Auth == false {
		return true
	}

	// 检查 Authorization 头
	authHeader := r.Header.Get("Authorization")
	expectedAuth := fmt.Sprintf("Bearer %v", config.Server.Auth)

	// 比较认证信息
	return authHeader == expectedAuth
}

// 认证中间件 - 检查请求的认证信息
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 如果认证已启用且验证失败
		if !validateAuth(r) {
			// 获取客户端IP并记录失败
			clientIP := getAuthClientIP(r)
			fmt.Printf("认证失败 - IP: %s, Auth: %s\n",
				clientIP, r.Header.Get("Authorization"))

			// 返回禁止访问错误
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// 认证成功，调用下一个处理函数
		next(w, r)
	}
}

// 处理内容直接访问，类似于 server-node 中的 /content/:id 路由
func handleContent(w http.ResponseWriter, r *http.Request) {
	// 从路径中提取ID
	idStr := strings.TrimPrefix(r.URL.Path, config.Server.Prefix+"/content/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid content ID", http.StatusBadRequest)
		return
	}

	// 获取room参数
	room := r.URL.Query().Get("room")

	// 在消息队列中查找对应条目
	found := false

	for _, msg := range messageQueue.List {
		// 只处理接收类型的消息
		if msg.Event == "receive" {
			// 检查是否为文本类型消息且ID和房间匹配
			if msg.Data.TextReceive != nil &&
				msg.Data.TextReceive.ID == id &&
				msg.Data.TextReceive.Room == room {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				content := html.UnescapeString(msg.Data.TextReceive.Content)
				if !strings.HasSuffix(content, "\n") {
					content += "\n"
				}
				w.Write([]byte(content))
				found = true
				break
			}

			// 检查是否为文件类型消息且ID和房间匹配
			if msg.Data.FileReceive != nil &&
				msg.Data.FileReceive.ID == id &&
				msg.Data.FileReceive.Room == room {
				// 文件类型，重定向到文件URL
				uuid := msg.Data.FileReceive.Cache

				// 构建文件URL
				scheme := getScheme(r)

				fileURL := fmt.Sprintf("%s://%s%s/file/%s",
					scheme,
					r.Host,
					config.Server.Prefix,
					uuid)

				http.Redirect(w, r, fileURL, http.StatusFound)
				found = true
				break
			}
		}
	}

	// 如果未找到对应消息
	if !found {
		http.NotFound(w, r)
	}
}

// 获取请求的协议 (http/https)
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}

// 修改现有的处理函数，使其返回内容URL
func enhanceHandleText(original http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 获取 messageQueue 的当前大小作为即将添加的消息的 ID
		nextID := messageQueue.nextid
		room := r.URL.Query().Get("room")

		// 调用原始处理函数
		original(w, r)

		// 如果状态码已经不是 200，说明原始处理函数已经处理了错误
		if w.Header().Get("Status") != "" && w.Header().Get("Status") != "200 OK" {
			return
		}

		// 构造内容访问URL
		scheme := getScheme(r)
		contentURL := fmt.Sprintf("%s://%s%s/content/%d",
			scheme, r.Host, config.Server.Prefix, nextID)

		// 如果有房间参数，添加到URL
		if room != "" {
			contentURL += fmt.Sprintf("?room=%s", room)
		}

		// 返回带URL的响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":   200,
			"msg":    "",
			"result": map[string]interface{}{"url": contentURL},
		})
	}
}

// 修改现有的文件上传完成处理函数，使其返回内容URL
func enhanceHandleFinish(original http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 获取UUID和房间参数
		// uuid := strings.TrimPrefix(r.URL.Path, config.Server.Prefix+"/upload/finish/")
		room := r.URL.Query().Get("room")

		// 调用原始处理函数，它会创建消息并广播
		original(w, r)

		// 请求处理完成后，构造与Node.js版兼容的URL响应
		// 获取最新添加的消息ID
		id := messageQueue.nextid - 1

		// 构造内容访问URL
		scheme := getScheme(r)
		contentURL := fmt.Sprintf("%s://%s%s/content/%d",
			scheme, r.Host, config.Server.Prefix, id)

		if room != "" {
			contentURL += fmt.Sprintf("?room=%s", room)
		}

		// 返回带URL的响应 - 与Node.js版本完全一致的格式
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"msg":  "",
			"result": map[string]interface{}{
				"url": contentURL,
			},
		})
	}
}
