// siteserver 在本机预览官网静态页面，由 make site 调用。
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:4173", "监听地址")
	dir := flag.String("dir", "site", "官网目录")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("官网预览：http://%s\n", listener.Addr())
	files := http.FileServer(http.Dir(*dir))
	// 预览时每次都向服务端校验，避免浏览器拿缓存的旧脚本配新页面。
	log.Fatal(http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})))
}
