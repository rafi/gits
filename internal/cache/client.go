package cache

import (
	"fmt"
	"time"

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

func NewCacheClient(name string, ttl time.Duration) (Cacher, error) {
	switch Client(name) {
	case ClientFile:
		return newCacheFile(ttl), nil
	default:
		return nil, fmt.Errorf("unknown cache client: %s", name)
	}
}
