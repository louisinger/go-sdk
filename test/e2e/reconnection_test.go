package e2e

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReconnection simulates a reconnection scenario by stopping the server and restarting it.
func TestReconnection(t *testing.T) {
	ctx := t.Context()
	alice := setupClient(t)

	// open streams
	aliceTxStream := alice.GetTransactionEventChannel(ctx)
	aliceVtxoStream := alice.GetVtxoEventChannel(ctx)


	// stop the arkd container
	err := exec.Command("docker", "stop", "arkd").Run()
	require.NoError(t, err)

	time.Sleep(3 * time.Second)

	// restart the arkd container
	err = exec.Command("docker", "start", "arkd").Run()
	require.NoError(t, err)

	time.Sleep(3 * time.Second)

	// unlock arkd 
	err = exec.Command("docker", "exec", "arkd", "arkd", "wallet", "unlock", "--password", password).Run()
	require.NoError(t, err)

	time.Sleep(20 * time.Second)

	// faucet offchain
	faucetOffchain(t, alice, 0.002)

	// the stream should receive 
	<-aliceTxStream
	<-aliceVtxoStream
}