package main

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gliderlabs/ssh"
)

type Runnable interface {
	Env() []string
	Setenv(k string, v string)
	Stdin() io.Writer
	Stdout() io.Reader
	Stderr() io.Reader
	ExitStatus() int
	Run() error
	IsPty() bool
	Resize(width, height int)
}

func Example() {
	ssh.Handle(func(sess ssh.Session) {
		client, err := client.NewEnvClient()
		if err != nil {
			panic(err)
		}
		runnable, err := NewDockerRunnable(client, &container.Config{})
		err = Attach(sess, runnable)
		if err != nil {
			panic(err)
		}
		sess.Exit(runnable.ExitStatus())
	})
}

func Attach(sess ssh.Session, runnable Runnable) error {
	ptyReq, winCh, _ := sess.Pty()

	// IO
	go func() {
		io.Copy(runnable.Stdin(), sess)
	}()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(sess, runnable.Stdout())
	}()
	if !runnable.IsPty() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			io.Copy(sess.Stderr(), runnable.Stderr())
		}()
	}
	// Environment
	for _, v := range sess.Environ() {
		kvp := strings.SplitN(v, "=", 2)
		if len(kvp) > 1 {
			runnable.Setenv(kvp[0], kvp[1])
		}
	}

	// Interactive
	if runnable.IsPty() {
		// Window resizing
		go func() {
			for win := range winCh {
				runnable.Resize(win.Width, win.Height)
			}
		}()
		// Terminal type
		runnable.Setenv("TERM", ptyReq.Term)
		// Terminal modes... TODO
	}

	err := runnable.Run()
	if err != nil {
		return err
	}
	return nil
}

type DockerRunnable struct {
	client     *client.Client
	env        []string
	exitStatus int

	containerCfg *container.Config
	containerID  string

	sync.Mutex
}

func NewDockerRunnable(client *client.Client, cfg *container.Config) (runnable *DockerRunnable, err error) {
	run := &DockerRunnable{
		client:       client,
		containerCfg: cfg,
	}
	ctx := context.Background()
	res, err := client.ContainerCreate(ctx, cfg, nil, nil, "")
	if err != nil {
		return nil, err
	}
	run.containerID = res.ID
	return run, nil
}

func (dr *DockerRunnable) Env() []string {
	dr.Lock()
	defer dr.Unlock()
	return dr.env
}
func (dr *DockerRunnable) Setenv(k string, v string) {
	dr.Lock()
	defer dr.Unlock()
	dr.env = append(dr.env, strings.Join([]string{k, v}, "="))
}
func (dr *DockerRunnable) Stdin() io.Writer {
	return nil
}
func (dr *DockerRunnable) Stdout() io.Reader {
	return nil
}
func (dr *DockerRunnable) Stderr() io.Reader {
	return nil
}
func (dr *DockerRunnable) ExitStatus() int {
	dr.Lock()
	defer dr.Unlock()
	return dr.exitStatus
}
func (dr *DockerRunnable) Run() error {
	return nil
}
func (dr *DockerRunnable) IsPty() bool {
	dr.Lock()
	defer dr.Unlock()
	return dr.containerCfg.Tty
}
func (dr *DockerRunnable) Resize(width, height int) {
	dr.Lock()
	defer dr.Unlock()
	ctx := context.Background()
	dr.client.ContainerResize(ctx, dr.containerID, types.ResizeOptions{
		Height: uint(height),
		Width:  uint(width),
	})
}
