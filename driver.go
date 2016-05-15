package main

import (
	"sync"

	"fmt"
	"github.com/jacobsa/fuse"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/jacobsa/fuse/fuseutil"
	g "github.com/monder/goofys-docker/internal"
	"path/filepath"

	"github.com/docker/go-plugins-helpers/volume"
)

type s3Bucket struct {
	fs          *fuse.MountedFileSystem
	connections int
}

type s3Driver struct {
	root    string
	buckets map[string]*s3Bucket
	m       *sync.Mutex
}

func newS3Driver(root string) s3Driver {
	return s3Driver{
		root:    root,
		buckets: map[string]*s3Bucket{},
		m:       &sync.Mutex{},
	}
}

func (d s3Driver) Create(r volume.Request) volume.Response {
	// TODO volume already exists
	return volume.Response{}
}
func (d s3Driver) Get(r volume.Request) volume.Response {
	return volume.Response{}
}
func (d s3Driver) List(r volume.Request) volume.Response {
	return volume.Response{}
}

func (d s3Driver) Remove(r volume.Request) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()
	bucket := strings.SplitN(r.Name, "/", 2)[0]

	if s, ok := d.buckets[bucket]; ok {
		if s.connections <= 1 {
			delete(d.buckets, bucket)
		}
	}
	return volume.Response{}
}

func (d s3Driver) Path(r volume.Request) volume.Response {
	return volume.Response{
		Mountpoint: d.mountpoint(r.Name),
	}
}

func (d s3Driver) Mount(r volume.Request) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()

	bucket := strings.SplitN(r.Name, "/", 2)[0]

	log.Printf("Mounting volume %s on %s\n", r.Name, d.mountpoint(bucket))

	s, ok := d.buckets[bucket]
	if ok && s.connections > 0 {
		s.connections++
		return volume.Response{Mountpoint: d.mountpoint(r.Name)}
	}

	fi, err := os.Lstat(d.mountpoint(bucket))

	if os.IsNotExist(err) {
		if err := os.MkdirAll(d.mountpoint(bucket), 0755); err != nil {
			return volume.Response{Err: err.Error()}
		}
	} else if err != nil {
		return volume.Response{Err: err.Error()}
	}

	if fi != nil && !fi.IsDir() {
		return volume.Response{Err: fmt.Sprintf("%v already exist and it's not a directory", d.mountpoint(bucket))}
	}

	fs, err := d.mountBucket(d.mountpoint(bucket), bucket)
	if err != nil {
		return volume.Response{Err: err.Error()}
	}

	d.buckets[bucket] = &s3Bucket{fs: fs, connections: 1}

	return volume.Response{Mountpoint: d.mountpoint(r.Name)}
}

func (d s3Driver) Unmount(r volume.Request) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()

	bucket := strings.SplitN(r.Name, "/", 2)[0]

	log.Printf("Unmounting volume %s from %s\n", r.Name, d.mountpoint(bucket))

	if s, ok := d.buckets[bucket]; ok {
		if s.connections == 1 {
			mountpoint := d.mountpoint(bucket)
			fuse.Unmount(mountpoint)
			os.Remove(mountpoint)
		}
		s.connections--
	} else {
		return volume.Response{Err: fmt.Sprintf("Unable to find volume mounted on %s", d.mountpoint(bucket))}
	}

	return volume.Response{}
}

func (d *s3Driver) mountpoint(name string) string {
	return filepath.Join(d.root, name)
}

func (d *s3Driver) mountBucket(mountpoint string, name string) (*fuse.MountedFileSystem, error) {
	// TODO check if already mounted
	if err := os.MkdirAll(filepath.Dir(mountpoint), 0755); err != nil {
		return nil, err
	}

	flags := &g.FlagStorage{
		Uid: 500,
		Gid: 500,
	}
	goofys := g.NewGoofys(
		name,
		&aws.Config{
			S3ForcePathStyle: aws.Bool(true),
			Region:           aws.String("eu-west-1"), //TODO
		},
		flags,
	)
	if goofys == nil {
		err := fmt.Errorf("Mount: initialization failed")
		return nil, err
	}
	server := fuseutil.NewFileSystemServer(goofys)

	mountCfg := &fuse.MountConfig{
		FSName:                  name,
		Options:                 map[string]string{"allow_other": ""},
		DisableWritebackCaching: true,
	}

	mfs, err := fuse.Mount(mountpoint, server, mountCfg)
	if err != nil {
		err = fmt.Errorf("Mount: %v", err)
		return nil, err
	}

	return mfs, nil
}
