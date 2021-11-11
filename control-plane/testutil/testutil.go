package testutil

import (
	"testing"

	capi "github.com/hashicorp/consul/api"
	ctestutil "github.com/hashicorp/consul/sdk/testutil"
	"github.com/stretchr/testify/require"
)

func NewTestServer(t *testing.T, cb ctestutil.ServerConfigCallback) *ctestutil.TestServer {
	server, err := ctestutil.NewTestServerConfigT(t, cb)
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	server.WaitForLeader(t)
	server.WaitForActiveCARoot(t)
	server.WaitForServiceIntentions(t)
	server.WaitForSerfCheck(t)
	return server
}

func NewTestServerClient(t *testing.T, cb ctestutil.ServerConfigCallback) *capi.Client {
	server := NewTestServer(t, cb)
	client, err := capi.NewClient(&capi.Config{
		Address: server.HTTPAddr,
	})
	require.NoError(t, err)
	return client
}
