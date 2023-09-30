package storage

type Storage interface {
	WriteFile(filePath string, contents []byte) error
	HasFile(filePath string) (bool, error)
	RemoveFile(filePath string) error
}
