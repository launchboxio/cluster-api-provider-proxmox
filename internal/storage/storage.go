package storage

type Storage interface {
	WriteFile(filePath string, contents []byte) error
}
