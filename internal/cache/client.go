package cache

import (
	"fmt"

	"github.com/rafi/gits/domain"
)

type Client string

const (
	ClientFile Client = "file"
)

type Cacher interface {
	Get(key string, project *domain.Project) (bool, error)
	Save(key string, project domain.Project) error
	Flush(project domain.Project) error
}

func NewCacheClient(name string) (Cacher, error) {
	switch Client(name) {
	case ClientFile:
		return newCacheFile()
	default:
		return nil, fmt.Errorf("unknown cache client: %s", name)
	}
}
