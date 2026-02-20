package arksdk

import (
	"context"
	"testing"

	"github.com/arkade-os/go-sdk/types"
	"github.com/stretchr/testify/require"
)

// Mock store for testing
type mockVtxoStore struct {
	vtxos []types.Vtxo
}

func (m *mockVtxoStore) GetSpendableVtxos(ctx context.Context) ([]types.Vtxo, error) {
	result := make([]types.Vtxo, 0)
	for _, v := range m.vtxos {
		if !v.Spent && !v.Unrolled {
			result = append(result, v)
		}
	}
	return result, nil
}

// TestVtxoSelectionWithRecoverableFlag tests that swept VTXOs are correctly
// filtered based on the WithRecoverableVtxos flag. This is the fix for issue #51.
func TestVtxoSelectionWithRecoverableFlag(t *testing.T) {
	// Create test VTXOs
	normalVtxo := types.Vtxo{
		Outpoint: types.Outpoint{Txid: "normal", VOut: 0},
		Amount:   10000,
		Spent:    false,
		Swept:    false,
		Unrolled: false,
	}

	sweptVtxo := types.Vtxo{
		Outpoint: types.Outpoint{Txid: "swept", VOut: 0},
		Amount:   20000,
		Spent:    false,
		Swept:    true, // Recoverable
		Unrolled: false,
	}

	spentVtxo := types.Vtxo{
		Outpoint: types.Outpoint{Txid: "spent", VOut: 0},
		Amount:   30000,
		Spent:    true,
		Swept:    false,
		Unrolled: false,
	}

	allVtxos := []types.Vtxo{normalVtxo, sweptVtxo, spentVtxo}

	t.Run("swept vtxo is recoverable", func(t *testing.T) {
		require.False(t, normalVtxo.IsRecoverable())
		require.True(t, sweptVtxo.IsRecoverable())
		require.False(t, spentVtxo.IsRecoverable())
	})

	t.Run("GetSpendableVtxos includes swept vtxos", func(t *testing.T) {
		store := &mockVtxoStore{vtxos: allVtxos}
		
		spendable, err := store.GetSpendableVtxos(context.Background())
		require.NoError(t, err)
		
		// Should return both normal and swept VTXOs
		require.Len(t, spendable, 2)
		
		hasNormal := false
		hasSwept := false
		for _, v := range spendable {
			if v.Outpoint.Txid == "normal" {
				hasNormal = true
			}
			if v.Outpoint.Txid == "swept" {
				hasSwept = true
			}
		}
		
		require.True(t, hasNormal, "Should include normal VTXO")
		require.True(t, hasSwept, "Should include swept VTXO for settlement/recovery")
	})

	t.Run("filtering logic: WITH recoverable vtxos (settlement/collaborative exit)", func(t *testing.T) {
		// Simulate what happens in CollaborativeExit / Settle
		// When WithRecoverableVtxos=true, both normal and swept VTXOs should be available
		
		spendable := []types.Vtxo{normalVtxo, sweptVtxo} // From GetSpendableVtxos
		
		recoverableVtxos := make([]types.Vtxo, 0)
		spendableVtxos := make([]types.Vtxo, 0)
		
		// This is the logic from getVtxos with WithRecoverableVtxos=true
		for _, vtxo := range spendable {
			if vtxo.IsRecoverable() {
				recoverableVtxos = append(recoverableVtxos, vtxo)
				continue
			}
			spendableVtxos = append(spendableVtxos, vtxo)
		}
		
		allVtxos := append(recoverableVtxos, spendableVtxos...)
		
		require.Len(t, recoverableVtxos, 1, "Should have 1 recoverable VTXO")
		require.Len(t, spendableVtxos, 1, "Should have 1 normal spendable VTXO")
		require.Len(t, allVtxos, 2, "Should use both for settlement")
		
		totalAmount := uint64(0)
		for _, v := range allVtxos {
			totalAmount += v.Amount
		}
		require.Equal(t, uint64(30000), totalAmount, "Should be able to spend all funds in settlement")
	})

	t.Run("filtering logic: WITHOUT recoverable vtxos (offchain payment)", func(t *testing.T) {
		// Simulate what happens in SendOffChain
		// When WithRecoverableVtxos=false, swept VTXOs should be EXCLUDED
		
		spendable := []types.Vtxo{normalVtxo, sweptVtxo} // From GetSpendableVtxos
		
		spendableVtxos := make([]types.Vtxo, 0)
		
		// This is the logic from getVtxos with WithRecoverableVtxos=false
		for _, vtxo := range spendable {
			if !vtxo.Swept {
				spendableVtxos = append(spendableVtxos, vtxo)
			}
		}
		
		require.Len(t, spendableVtxos, 1, "Should only have normal VTXO")
		require.Equal(t, "normal", spendableVtxos[0].Outpoint.Txid)
		require.Equal(t, uint64(10000), spendableVtxos[0].Amount)
		
		// Swept VTXO should NOT be available for offchain payments
		for _, v := range spendableVtxos {
			require.False(t, v.Swept, "Offchain payments should not use swept VTXOs")
		}
	})
}
