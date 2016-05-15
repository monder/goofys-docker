package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/docker/go-plugins-helpers/volume"
	_ "golang.org/x/net/context"
)

const (
	socketAddress = "/run/docker/plugins/s3volume.sock"
)

var (
	defaultPath = filepath.Join(volume.DefaultDockerRootDirectory, "s3volume")
	root        = flag.String("root", defaultPath, "Docker volumes root directory")
	uid         = flag.String("uid", "500", "Default uid to own files")
	gid         = flag.String("gid", "500", "Default gid to own files")
)

func main() {
	flag.Parse()

	d := newS3Driver(*root)
	h := volume.NewHandler(d)

	fmt.Printf("Listening on %s\n", socketAddress)
	fmt.Println(h.ServeUnix("wheel", socketAddress))
}
