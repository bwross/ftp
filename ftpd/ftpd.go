package main

import (
	"fmt"
	"os"

	"github.com/bwross/ftp"
)

func main() {
	server := ftp.Server{
		Handler: &ftp.FileHandler{
			Authorizer: ftp.AuthAny,
			FileSystem: &ftp.LocalFileSystem{},
		},
	}
	if len(os.Args) > 1 {
		server.Addr = os.Args[1]
	}
	err := server.ListenAndServe()
	fmt.Println(err)
}
