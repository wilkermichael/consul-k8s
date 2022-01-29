package catalog

import (
	"sync"

	"github.com/hashicorp/consul/api"
)

const (
	TestConsulK8STag = "k8s"
)

// testSyncer implements Syncer for tests, giving easy access to the
// set of registrations.
type testSyncer struct {
	sync.RWMutex  // Lock should be held while accessing Registrations
	registrations []*api.CatalogRegistration
}

// Sync implements Syncer
func (s *testSyncer) Sync(rs []*api.CatalogRegistration) {
	s.Lock()
	defer s.Unlock()
	s.registrations = rs
}

func (s *testSyncer) Registrations() []*api.CatalogRegistration {
	s.RLock()
	defer s.RUnlock()
	return s.registrations
}

func newTestSyncer() *testSyncer {
	return &testSyncer{}
}
