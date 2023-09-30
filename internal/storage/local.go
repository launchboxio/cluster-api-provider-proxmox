package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/luthermonson/go-proxmox"
	"path/filepath"
	"strings"
)

type Local struct {
	Client    *proxmox.Client
	NodeName  string
	Directory string
	node      *proxmox.Node
	termProxy *proxmox.VNC
	conn      *websocket.Conn
	send      chan string
	recv      chan string
	errs      chan error
	closer    func() error
}

func NewLocal(client *proxmox.Client, node string, directory string) (*Local, error) {
	local := &Local{
		Client:    client,
		NodeName:  node,
		Directory: filepath.Join(directory, "snippets"),
	}
	err := local.init()
	return local, err
}

func (l *Local) init() error {
	node, err := l.Client.Node(l.NodeName)
	if err != nil {
		return err
	}
	if l.node == nil {
		l.node = node
	}
	vnc, err := node.TermProxy()
	if err != nil {
		return err
	}
	l.termProxy = vnc
	send, recv, errs, closer, err := l.node.VNCWebSocket(l.termProxy)
	l.send = send
	l.recv = recv
	l.errs = errs
	l.closer = closer
	return nil
}

func (l *Local) Close() error {
	return l.closer()
}

func (l *Local) WriteFile(filePath string, contents []byte) error {
	if err := l.RemoveFile(filePath); err != nil {
		return err
	}

	chunks := chunkString(string(contents), 2000)
	for _, chunk := range chunks {
		b64chunk := base64.StdEncoding.EncodeToString([]byte(chunk))
		_, _, err := l.sendCommand(context.TODO(), fmt.Sprintf("echo %s | base64 -d >> %s", b64chunk, filepath.Join(l.Directory, filePath)))
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *Local) HasFile(filePath string) (bool, error) {
	out, _, err := l.sendCommand(context.TODO(), fmt.Sprintf("stat %s", filepath.Join(l.Directory, filePath)))
	if err != nil {
		return false, err
	}
	// TODO: Check the output to verify exit code
	return len(out) > 1, nil
}

func (l *Local) RemoveFile(filePath string) error {
	command := fmt.Sprintf("rm -f %s", filepath.Join(l.Directory, filePath))
	_, _, err := l.sendCommand(context.TODO(), command)
	return err
}

func (l *Local) sendCommand(ctx context.Context, command string) ([]string, int, error) {
	var out []string
	done := make(chan error, 1)

	go func() {
		defer close(done)

		for {
			select {
			case recvErr := <-l.errs:
				if recvErr != nil {
					done <- recvErr
					return
				}

			case msg := <-l.recv:
				if msg != "" {
					if strings.HasSuffix(strings.TrimSpace(msg), ":~#") {
						done <- nil
						return
					}
				}
			}
		}
	}()

	l.send <- command

	select {
	case err := <-done:
		return out, 0, err
	case <-ctx.Done():
		return out, -1, errors.New("context deadline exceeded")
	}
}

func chunkString(s string, chunkSize int) []string {
	var chunks []string
	runes := []rune(s)
	if len(runes) == 0 {
		return []string{s}
	}
	for i := 0; i < len(runes); i += chunkSize {
		nn := i + chunkSize
		if nn > len(runes) {
			nn = len(runes)
		}
		chunks = append(chunks, string(runes[i:nn]))
	}
	return chunks
}
