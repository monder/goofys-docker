package main

import (
	"sync"

	"fmt"
	"github.com/jacobsa/fuse"
	"log"
	"os"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/jacobsa/fuse/fuseutil"
	g "github.com/monder/goofys-docker/internal"
	"path/filepath"

	"github.com/docker/go-plugins-helpers/volume"
)

type s3Driver struct {
	root        string
	connections map[string]int
	volumes     map[string]bool
	m           *sync.Mutex
}

func newS3Driver(root string) s3Driver {
	return s3Driver{
		root:        root,
		connections: map[string]int{},
		volumes:     map[string]bool{},
		m:           &sync.Mutex{},
	}
}

func (d s3Driver) Create(r volume.Request) volume.Response {
	if _, exists := d.volumes[r.Name]; exists {
		return volume.Response{Err: "Volume already exists"}
	}
	d.volumes[r.Name] = true
	return volume.Response{}
}

func (d s3Driver) Get(r volume.Request) volume.Response {
	if _, exists := d.volumes[r.Name]; exists {
		return volume.Response{
			Volume: &volume.Volume{
				Name:       r.Name,
				Mountpoint: d.mountpoint(r.Name),
			},
		}
	}
	return volume.Response{}
}

func (d s3Driver) List(r volume.Request) volume.Response {
	var volumes []*volume.Volume
	for k := range d.volumes {
		volumes = append(volumes, &volume.Volume{
			Name:       k,
			Mountpoint: d.mountpoint(k),
		})
	}
	return volume.Response{
		Volumes: volumes,
	}
}

func (d s3Driver) Remove(r volume.Request) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()
	bucket := strings.SplitN(r.Name, "/", 2)[0]

	count, exists := d.connections[bucket]
	if exists && count < 1 {
		delete(d.connections, bucket)
	}
	delete(d.volumes, r.Name)
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

	count, exists := d.connections[bucket]
	if exists && count > 0 {
		d.connections[bucket] = count + 1
		return volume.Response{Mountpoint: d.mountpoint(r.Name)}
	}

	fi, err := os.Lstat(d.mountpoint(bucket))

	if os.IsNotExist(err) {
		if err := os.MkdirAll(d.mountpoint(bucket), 0755); err != nil {
			return volume.Response{Err: err.Error()}
		}
	} else if err != nil {
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOTCONN {
			// Crashed previously? Remount
			log.Println(e.Err)
			log.Println(e.Err == syscall.ENOTCONN)
			fuse.Unmount(d.mountpoint(bucket))
		} else {
			return volume.Response{Err: err.Error()}
		}
	}

	if fi != nil && !fi.IsDir() {
		return volume.Response{Err: fmt.Sprintf("%v already exist and it's not a directory", d.mountpoint(bucket))}
	}

	err = d.mountBucket(d.mountpoint(bucket), bucket)
	if err != nil {
		return volume.Response{Err: err.Error()}
	}

	d.connections[bucket] = 1

	return volume.Response{Mountpoint: d.mountpoint(r.Name)}
}

func (d s3Driver) Unmount(r volume.Request) volume.Response {
	d.m.Lock()
	defer d.m.Unlock()

	bucket := strings.SplitN(r.Name, "/", 2)[0]

	log.Printf("Unmounting volume %s from %s\n", r.Name, d.mountpoint(bucket))

	if count, exists := d.connections[bucket]; exists {
		if count == 1 {
			mountpoint := d.mountpoint(bucket)
			fuse.Unmount(mountpoint)
			os.Remove(mountpoint)
		}
		d.connections[bucket] = count - 1
	} else {
		return volume.Response{Err: fmt.Sprintf("Unable to find volume mounted on %s", d.mountpoint(bucket))}
	}

	return volume.Response{}
}

func (d *s3Driver) mountpoint(name string) string {
	return filepath.Join(d.root, name)
}

func (d *s3Driver) mountBucket(mountpoint string, name string) error {
	// TODO check if already mounted

	goofys := g.NewGoofys(
		name,
		&aws.Config{
			S3ForcePathStyle: aws.Bool(true),
			Region:           aws.String("eu-west-1"), //TODO
		},
		&g.FlagStorage{
		//Uid: 500,
		//Gid: 500,
		},
	)
	if goofys == nil {
		err := fmt.Errorf("Goofys: initialization failed")
		return err
	}
	server := fuseutil.NewFileSystemServer(goofys)

	mountCfg := &fuse.MountConfig{
		FSName:                  name,
		Options:                 map[string]string{"allow_other": ""},
		DisableWritebackCaching: true,
	}

	_, err := fuse.Mount(mountpoint, server, mountCfg)
	if err != nil {
		err = fmt.Errorf("Mount: %v", err)
		return err
	}

	return nil
}
