package main

import (
	"flag"
	"fmt"

	"github.com/bwross/ftp"
)

func main() {
	addr := flag.String("addr", "", "addr to bind control channel")
	host := flag.String("host", "", "host to bind passive data channels")

	flag.Parse()

	server := ftp.Server{
		Addr: *addr,
		Host: *host,
		Handler: &ftp.FileHandler{
			Authorizer: ftp.AuthAny,
			FileSystem: &ftp.LocalFileSystem{},
		},
	}
	err := server.ListenAndServe()
	fmt.Println(err)
}
