package lib

/**
*** FILE: util.go
***   handle misc tools
**/

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"image"
	"image/jpeg"

	// _ "image/jpeg"
	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"os"

	"github.com/google/uuid"
	"golang.org/x/image/draw"
)

// ------- get info from ua
// IEC units for file size
const (
	_   = iota
	_KB = 1 << (10 * iota)
	_MB
	_GB
	_TB
	// _PB
	// _EB
	// _ZB
	// _YB
)

// ------- get info from ua
// X-Real-IP: 1.2.3.4
// X-Real-Port: 4759
// X-Forwarded-For: 1.2.3.4
// X-NginX-Proxy: true
// X-Forwarded-Proto: https

// func get_remote(r *http.Request) (ip, port string) {
// 	ip, port, _ = net.SplitHostPort(r.RemoteAddr)
// 	real_ip := r.Header.Get("X-Real-IP")   //ip only
// 	real_pt := r.Header.Get("X-Real-Port") //port only
// 	fmt.Println("==ip, port, remote:", ip, port, real_ip, real_pt)
// 	if real_ip != "" {
// 		ip = real_ip
// 	}
// 	if real_pt != "" {
// 		port = real_pt
// 	}

// 	return
// }

// func get_UA(r *http.Request) string {
// 	ua := r.Header.Get("User-Agent")
// 	parser := uaparser.NewFromSaved()
// 	client := parser.Parse(ua)
// 	return fmt.Sprintf("%s / %s", client.Os.Family, client.UserAgent.Family)
// }

// ------- hash, uuid
func gen_UUID() string {
	return uuid.New().String()
}

func random_bytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}

// ------ gen thumbnail
func gen_thumbnail(imgPath string) (string, error) {
	fmt.Println("--gen_thumbnail:", imgPath)
	imgFile, err := os.Open(imgPath)
	if err != nil {
		return "", err
	}
	defer imgFile.Close()

	img, _, err := image.Decode(imgFile)
	// 	img, err = png.Decode(imgFile)

	if err != nil {
		fmt.Println("-- image.decode fail")
		return "", err
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y

	if min(width, height) > 64 {
		ratio := 64.0 / float64(min(width, height))
		width = int(float64(width) * ratio)
		height = int(float64(height) * ratio)
	}

	thumbnail := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), img, bounds, draw.Over, nil)

	buffer := new(bytes.Buffer)

	// err = png.Encode(buffer, thumbnail)
	err = jpeg.Encode(buffer, thumbnail, &jpeg.Options{Quality: 70})
	if err != nil {
		return "", err
	}

	imgBase64 := base64.StdEncoding.EncodeToString(buffer.Bytes())
	// return fmt.Sprintf("data:image/png;base64,%s", imgBase64), nil
	return fmt.Sprintf("data:image/jpeg;base64,%s", imgBase64), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

// func get_all_msg(history *History) []interface{} {
// 	var msg_list []interface{}

// 	for _, receiveItem := range history.Receive {
// 		if receiveItem.TextReceive != nil {
// 			msg_list = append(msg_list, *receiveItem.TextReceive)
// 		} else if receiveItem.FileReceive != nil {
// 			msg_list = append(msg_list, *receiveItem.FileReceive)
// 		}
// 	}

// 	return msg_list
// }
