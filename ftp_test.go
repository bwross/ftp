package ftp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"
)

// Not a real test. This is just to aid in manual testing until the client is
// done.
func TestServer(t *testing.T) {
	ftp := Server{
		Addr:  ":2121",
		Debug: true,
		Handler: &FileHandler{
			Authorizer: AuthAny,
			FileSystem: &LocalFileSystem{},
		},
	}

	go func() {
		fmt.Println("Starting FTP server...")
		err := ftp.ListenAndServe()
		panic(err)
	}()

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(123),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             now,
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign & x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:           true,
		MaxPathLenZero: true,
	}

	key, err := rsa.GenerateKey(rand.Reader, 512)
	if err != nil {
		panic(err)
	}

	xcert, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}

	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: xcert,
		}),
		pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		}),
	)
	if err != nil {
		panic(err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		CipherSuites: []uint16{
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		},
	}

	ftps := Server{
		Addr:  ":9990",
		TLS:   cfg,
		Debug: true,
		Handler: &FileHandler{
			Authorizer: AuthAny,
			FileSystem: &LocalFileSystem{},
		},
	}

	fmt.Println("Starting FTPS server...")
	err = ftps.ListenAndServe()
	panic(err)
}
