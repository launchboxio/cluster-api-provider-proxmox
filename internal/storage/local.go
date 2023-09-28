package storage

import (
	"fmt"
	"github.com/luthermonson/go-proxmox"
	"path/filepath"
)

type Local struct {
	Client    *proxmox.Client
	Node      string
	Directory string
}

func NewLocal(client *proxmox.Client, node string, directory string) *Local {
	return &Local{
		Client:    client,
		Node:      node,
		Directory: directory,
	}
}

func (l *Local) WriteFile(filePath string, contents []byte) error {
	node, err := l.Client.Node(l.Node)
	if err != nil {
		return err
	}

	vnc, err := node.TermProxy()
	if err != nil {
		return err
	}

	send, _, errs, close, err := node.VNCWebSocket(vnc)
	if err != nil {
		return err
	}

	defer close()

	var vncError error
	go func() {
		for {
			select {
			case err := <-errs:
				if err != nil {
					vncError = err
				}
			}
		}
	}()

	send <- fmt.Sprintf("echo %s > %s", string(contents), filepath.Join(l.Directory, filePath))
	return vncError
}
