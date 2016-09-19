package ftp

import (
	"fmt"
	"testing"
)

// Not a real test. This is just to aid in manual testing until the client is
// done.
func TestServer(t *testing.T) {
	server := Server{
		Addr:  ":2121",
		Debug: true,
		Handler: &FileHandler{
			Authorizer: AuthAny,
			FileSystem: &LocalFileSystem{},
		},
	}
	fmt.Println("Starting server...")
	err := server.ListenAndServe()
	fmt.Println(err)
}
