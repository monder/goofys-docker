package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/docker/go-plugins-helpers/volume"
	_ "golang.org/x/net/context"
)

const (
	socketAddress = "/run/docker/plugins/goofys.sock"
)

var (
	defaultPath = filepath.Join(volume.DefaultDockerRootDirectory, "goofys")
	root        = flag.String("root", defaultPath, "Docker volumes root directory")
)

func main() {
	flag.Parse()

	d := newS3Driver(*root)
	h := volume.NewHandler(d)

	fmt.Printf("Listening on %s\n", socketAddress)
	fmt.Println(h.ServeUnix("wheel", socketAddress))
}
