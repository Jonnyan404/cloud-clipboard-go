//go:build !embed
// +build !embed

package main

import (
	"flag"
	"fmt"
	"net/http"
)

// specify external static folder
var flg_external_static = flag.String("static", "./static", "Path to external static files")

func server_static(prefix string) {

	if *flg_external_static != "" {
		fmt.Println("-- serve from external static:", *flg_external_static)
		// use external static
		// http.Handle(prefix+"/", http.StripPrefix(prefix, http.FileServer(http.Dir(*flg_external_static))))
		http.Handle(prefix+"/", http.StripPrefix(prefix, compressionMiddleware(http.FileServer(http.Dir(*flg_external_static)))))
	}
}
