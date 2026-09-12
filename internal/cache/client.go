package cache

import (
	"github.com/rafi/gits/domain"
)

type Cacher interface {
	Get(key string, project *domain.Project) (bool, error)
	Save(key string, project domain.Project) error
	Flush(project domain.Project) error
}
