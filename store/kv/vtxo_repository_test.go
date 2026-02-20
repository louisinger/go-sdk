package kvstore

import (
	"context"
	"testing"

	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestGetSpendableVtxos_WithSweptVtxos tests that swept VTXOs are correctly
// handled by the store. This is a regression test for issue #51.
// 
// Context: Swept VTXOs (swept=true, spent=false) represent "recoverable coins"
// - They ARE spendable in settlement/collaborative exit (onchain)  
// - They are NOT spendable in offchain payments
//
// The bug was that GetSpendableVtxos excluded ALL swept VTXOs, preventing
// them from being used in any path.
func TestGetSpendableVtxos_WithSweptVtxos(t *testing.T) {
	ctx := context.Background()
	store, err := NewVtxoStore("", nil)
	require.NoError(t, err)
	defer store.Close()

	// Create test VTXOs with different states
	vtxos := []types.Vtxo{
		{
			Outpoint: types.Outpoint{Txid: "tx1", VOut: 0},
			Amount:   10000,
			Spent:    false,
			Swept:    false, // Normal spendable VTXO
			Unrolled: false,
		},
		{
			Outpoint: types.Outpoint{Txid: "tx2", VOut: 0},
			Amount:   20000,
			Spent:    false,
			Swept:    true, // Swept but not spent = RECOVERABLE
			Unrolled: false,
		},
		{
			Outpoint: types.Outpoint{Txid: "tx3", VOut: 0},
			Amount:   30000,
			Spent:    true, // Spent = not spendable
			Swept:    false,
			Unrolled: false,
		},
		{
			Outpoint: types.Outpoint{Txid: "tx4", VOut: 0},
			Amount:   40000,
			Spent:    false,
			Swept:    false,
			Unrolled: true, // Unrolled = not spendable
		},
	}

	// Add VTXOs to store
	count, err := store.AddVtxos(ctx, vtxos)
	require.NoError(t, err)
	require.Equal(t, 4, count)

	// Test GetSpendableVtxos - this currently excludes swept VTXOs (THE BUG)
	// After the fix, it should return VTXOs that can be used in coin selection,
	// which includes swept VTXOs for settlement/recovery paths
	spendable, err := store.GetSpendableVtxos(ctx)
	require.NoError(t, err)

	// BEFORE FIX: Returns only tx1 (10,000 sats) - excludes swept tx2
	// AFTER FIX: Should return tx1 and tx2 (30,000 sats total)
	// The filtering for offchain-only should happen at a higher level
	
	// Log current behavior
	t.Logf("GetSpendableVtxos returned %d VTXOs:", len(spendable))
	for _, v := range spendable {
		t.Logf("  - %s: amount=%d, swept=%v, spent=%v", v.Outpoint.Txid, v.Amount, v.Swept, v.Spent)
	}

	// This will FAIL before the fix and PASS after
	require.Len(t, spendable, 2, "Should return both normal and swept VTXOs")
	
	// Verify the correct VTXOs are returned
	hasNormalVtxo := false
	hasSweptVtxo := false
	
	for _, v := range spendable {
		if v.Outpoint.Txid == "tx1" {
			hasNormalVtxo = true
			require.False(t, v.Swept)
			require.False(t, v.Spent)
		}
		if v.Outpoint.Txid == "tx2" {
			hasSweptVtxo = true
			require.True(t, v.Swept)
			require.False(t, v.Spent)
			require.True(t, v.IsRecoverable(), "tx2 should be recoverable")
		}
	}
	
	require.True(t, hasNormalVtxo, "Should include normal spendable VTXO (tx1)")
	require.True(t, hasSweptVtxo, "Should include swept/recoverable VTXO (tx2) for settlement")
}
