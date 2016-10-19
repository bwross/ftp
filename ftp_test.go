package ftp

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestFTP(t *testing.T) {
	s := &Server{
		Addr: "localhost:0",
		Handler: &FileHandler{
			Authorizer: new(testAuth),
			FileSystem: newTestFS(),
		},
	}
	li, err := s.ListenAndServe(true)
	if err != nil {
		t.Fatal(err)
	}
	defer li.Close()

	c := &Client{
		Addr: li.Addr().String(),
	}
	defer c.Close()

	ok, err := c.Authorize("admin", "password1")
	if err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("login succeeded; should fail")
	}

	ok, err = c.Authorize("foo", "bar")
	if err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("login failed")
	}

	f, err := c.Create("foo.txt")
	if err != nil {
		t.Fatal(err)
	}

	f.Write([]byte("wow cool"))
	f.Close()

	f, err = c.Open("foo.txt")
	if err != nil {
		t.Fatal(err)
	}

	if b, err := ioutil.ReadAll(f); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(b, []byte("wow cool")) {
		t.Fatal("bad data:", string(b))
	}
}

func newTLS() *tls.Config {
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

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		CipherSuites: []uint16{
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		},
	}
}

// Not a real test. This is just to aid in manual testing until the client is
// done.
func testServer(t *testing.T) {
	ftp := Server{
		Addr:  ":2121",
		Debug: true,
		Handler: &FileHandler{
			FileSystem: &LocalFileSystem{},
		},
	}

	fmt.Println("Starting FTP server...")
	_, err := ftp.ListenAndServe(true)
	if err != nil {
		panic(err)
	}

	ftps := Server{
		Addr:  ":9990",
		TLS:   newTLS(),
		Debug: true,
		Handler: &FileHandler{
			FileSystem: &LocalFileSystem{},
		},
	}

	fmt.Println("Starting FTPS server...")
	_, err = ftps.ListenAndServe(false)
	panic(err)
}

type fileInfos []os.FileInfo

func (f fileInfos) Len() int           { return len(f) }
func (f fileInfos) Less(i, j int) bool { return f[i].Name() < f[j].Name() }
func (f fileInfos) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

type testFile struct {
	fs   testFS
	path string
	size int64
	mode os.FileMode
	time time.Time
	r    *bytes.Reader
	w    *bytes.Buffer
	list []os.FileInfo
}

func (f *testFile) Read(b []byte) (n int, err error) {
	if f.r == nil {
		return 0, errors.New("nope")
	}
	return f.r.Read(b)
}

func (f *testFile) Write(b []byte) (n int, err error) {
	if f.w == nil {
		f.w = new(bytes.Buffer)
	}
	return f.w.Write(b)
}

func (f *testFile) Close() error {
	if f.w != nil {
		f.r = bytes.NewReader(f.w.Bytes())
		f.size = f.r.Size()
		f.w = nil
		f.fs[f.path] = f
	}
	return nil
}

func (f *testFile) Readdir(n int) ([]os.FileInfo, error) {
	if n <= 0 {
		return f.readdir(), nil
	}
	if f.list == nil {
		f.list = f.readdir()
	}
	if len(f.list) == 0 {
		return nil, io.EOF
	}
	if n > len(f.list) {
		n = len(f.list)
	}
	fi := f.list[:n]
	f.list = f.list[n:]
	return []os.FileInfo(fi), nil
}

func (f *testFile) readdir() (fi fileInfos) {
	m := make(map[string]os.FileInfo)
	for k, v := range f.fs {
		if !strings.HasPrefix(k, f.path) {
			continue
		}
		suffix := strings.TrimPrefix(k, f.path)
		if suffix == "" {
			continue
		}
		parts := strings.Split(suffix, "/")
		name := parts[0]
		if len(parts) == 1 {
			m[name] = v
		} else if m[name] == nil {
			m[name] = &testFile{}
		}
	}
	for _, v := range m {
		fi = append(fi, v)
	}
	sort.Sort(fi)
	return fi
}

func (f *testFile) Seek(off int64, whence int) (int64, error) {
	if f.r == nil {
		return 0, errors.New("nope")
	}
	return f.r.Seek(off, whence)
}

func (f *testFile) Name() string       { return path.Base(f.path) }
func (f *testFile) Size() int64        { return f.size }
func (f *testFile) Mode() os.FileMode  { return f.mode }
func (f *testFile) ModTime() time.Time { return f.time }
func (f *testFile) IsDir() bool        { return f.mode.IsDir() }
func (f *testFile) Sys() interface{}   { return nil }

type testFS map[string]*testFile

func newTestFS() testFS {
	f := make(testFS)
	f.Mkdir("/")
	return f
}

func (testFS) path(p string) string { return path.Join("/", p) }

func (f testFS) Create(p string) (File, error) {
	tf := &testFile{
		fs:   f,
		path: f.path(p),
		mode: 0644,
		time: time.Now(),
	}
	return tf, nil
}

func (f testFS) Open(p string) (File, error) {
	tf := f[f.path(p)]
	if tf == nil {
		return nil, os.ErrNotExist
	}
	return tf, nil
}

func (f testFS) Mkdir(p string) error {
	if f[f.path(p)] != nil {
		return os.ErrExist
	}
	f[f.path(p)] = &testFile{
		fs:   f,
		path: f.path(p),
		mode: 0755 | os.ModeDir,
		time: time.Now(),
	}
	return nil
}

func (f testFS) Remove(p string) error {
	tf := f[f.path(p)]
	if tf == nil {
		return os.ErrNotExist
	}
	delete(f, f.path(p))
	return nil
}

func (f testFS) Rename(old, new string) error {
	tf := f[f.path(old)]
	if tf == nil {
		return os.ErrNotExist
	}
	delete(f, f.path(old))
	f[f.path(new)] = tf
	return nil
}

func (f testFS) Stat(p string) (os.FileInfo, error) {
	tf := f[f.path(p)]
	if tf == nil {
		return nil, os.ErrNotExist
	}
	return tf, nil
}

type testAuth struct{}

func (testAuth) Authorize(user, pass string) (bool, error) {
	return user == "foo" && pass == "bar", nil
}
