package storage

import (
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	v1 "k8s.io/api/core/v1"
)

type Sftp struct {
	Client *sftp.Client
}

func SftpFromSecret(secret *v1.Secret) (*Sftp, error) {
	config := &ssh.ClientConfig{
		User: string(secret.Data["user"]),
		Auth: []ssh.AuthMethod{
			ssh.Password(string(secret.Data["password"])),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", string(secret.Data["host"]), config)
	if err != nil {
		return nil, err
	}

	fs, err := sftp.NewClient(client)
	if err != nil {
		return nil, err
	}

	return &Sftp{Client: fs}, nil
}

func (s *Sftp) Close() error {
	return s.Client.Close()
}

func (s *Sftp) WriteFile(filePath string, contents []byte) error {
	dstFile, err := s.Client.Create(filePath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.Write(contents)
	return err
}
