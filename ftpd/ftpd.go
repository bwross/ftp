package main

import (
	"flag"
	"fmt"

	"github.com/igneous-systems/ftp"
)

func main() {
	addr := flag.String("addr", "", "addr to bind control channel")

	flag.Parse()

	server := ftp.Server{
		Addr: *addr,
		Handler: &ftp.FileHandler{
			FileSystem: &ftp.LocalFileSystem{},
		},
	}
	_, err := server.ListenAndServe(false)
	fmt.Println(err)
}
