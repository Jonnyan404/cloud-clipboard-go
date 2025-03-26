module github.com/yurenchen000/cloud-clipboard/server-go/cloud-clip

// module github.com/yurenchen000/cloud-clipboard/cloud-clip
// module cloud-clip

// replace cloud-clip => github.com/yurenchen000/cloud-clipboard/server-go@go-1.3
// replace cloud-clip => github.com/yurenchen000/cloud-clipboard/server-go go-1.3
// replace cloud-clip => github.com/yurenchen000/cloud-clipboard/server-go v0.0.1

go 1.22

require (
	github.com/andybalholm/brotli v1.1.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/klauspost/compress v1.17.11
	github.com/spaolacci/murmur3 v1.1.0
	github.com/ua-parser/uap-go v0.0.0-20241012191800-bbb40edc15aa
	golang.org/x/image v0.21.0
)

require (
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	gopkg.in/yaml.v2 v2.2.1 // indirect
)
