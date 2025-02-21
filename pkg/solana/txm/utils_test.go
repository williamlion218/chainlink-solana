package txm

import (
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortSignaturesAndResults(t *testing.T) {
	sig := []solana.Signature{
		{0}, {1}, {2}, {3},
	}

	statuses := []*rpc.SignatureStatusesResult{
		{ConfirmationStatus: rpc.ConfirmationStatusProcessed},
		{ConfirmationStatus: rpc.ConfirmationStatusConfirmed},
		nil,
		{ConfirmationStatus: rpc.ConfirmationStatusConfirmed, Err: "ERROR"},
	}

	_, _, err := SortSignaturesAndResults([]solana.Signature{}, statuses)
	require.Error(t, err)

	sig, statuses, err = SortSignaturesAndResults(sig, statuses)
	require.NoError(t, err)

	// new expected order [1, 0, 3, 2]
	assert.Equal(t, rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusConfirmed}, *statuses[0])
	assert.Equal(t, rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusProcessed}, *statuses[1])
	assert.Equal(t, rpc.SignatureStatusesResult{ConfirmationStatus: rpc.ConfirmationStatusConfirmed, Err: "ERROR"}, *statuses[2])
	assert.True(t, nil == statuses[3])

	assert.Equal(t, solana.Signature{1}, sig[0])
	assert.Equal(t, solana.Signature{0}, sig[1])
	assert.Equal(t, solana.Signature{3}, sig[2])
	assert.Equal(t, solana.Signature{2}, sig[3])
}

func TestSignatureList_AllocateWaitSet(t *testing.T) {
	sigs := signatureList{}
	assert.Equal(t, 0, sigs.Length())

	// can't set without pre-allocating
	assert.ErrorContains(t, sigs.Set(0, solana.Signature{}), "invalid index")

	// can't set on index that has already been set
	assert.Equal(t, 0, sigs.Allocate())
	assert.Equal(t, 1, sigs.Length())
	assert.NoError(t, sigs.Set(0, solana.Signature{1}))
	assert.ErrorContains(t, sigs.Set(0, solana.Signature{1}), "trying to set signature when already set")

	// waitgroup does not block on invalid index
	sigs.Wait(100000)

	// waitgroup blocks between allocate and set
	ind1 := sigs.Allocate()
	assert.Equal(t, 1, ind1)
	ind2 := sigs.Allocate()
	assert.Equal(t, 2, ind2)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		sigs.Wait(ind1)
		sigs.Wait(ind2)
		wg.Done()
	}()
	assert.NoError(t, sigs.Set(ind2, solana.Signature{1}))
	assert.NoError(t, sigs.Set(ind1, solana.Signature{1}))
	wg.Wait()
}

func TestSetTxConfig(t *testing.T) {
	cfg := TxConfig{}

	for _, v := range []SetTxConfig{
		SetTimeout(1 * time.Second),
		SetFeeBumpPeriod(2 * time.Second),
		SetBaseComputeUnitPrice(3),
		SetComputeUnitPriceMin(4),
		SetComputeUnitPriceMax(5),
		SetComputeUnitLimit(6),
	} {
		v(&cfg)
	}

	assert.Equal(t, 1*time.Second, cfg.Timeout)
	assert.Equal(t, 2*time.Second, cfg.FeeBumpPeriod)
	assert.Equal(t, uint64(3), cfg.BaseComputeUnitPrice)
	assert.Equal(t, uint64(4), cfg.ComputeUnitPriceMin)
	assert.Equal(t, uint64(5), cfg.ComputeUnitPriceMax)
	assert.Equal(t, uint32(6), cfg.ComputeUnitLimit)
}
