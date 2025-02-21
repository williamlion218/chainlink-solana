package client

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	mn "github.com/smartcontractkit/chainlink-solana/pkg/solana/client/multinode"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/monitor"
)

func TestClient_Reader_Integration(t *testing.T) {
	ctx := tests.Context(t)
	url := SetupLocalSolNode(t)
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privKey.PublicKey()
	FundTestAccounts(t, []solana.PublicKey{pubKey}, url)

	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()

	c, err := NewClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	// check balance
	bal, err := c.Balance(ctx, pubKey)
	assert.NoError(t, err)
	assert.Equal(t, uint64(100_000_000_000), bal) // once funds get sent to the system program it should be unrecoverable (so this number should remain > 0)

	// check SlotHeight
	slot0, err := c.SlotHeight(ctx)
	assert.NoError(t, err)
	assert.Greater(t, slot0, uint64(0))
	time.Sleep(time.Second)
	slot1, err := c.SlotHeight(ctx)
	assert.NoError(t, err)
	assert.Greater(t, slot1, slot0)

	// fetch recent blockhash
	hash, err := c.LatestBlockhash(ctx)
	assert.NoError(t, err)
	assert.NotEqual(t, hash.Value.Blockhash, solana.Hash{}) // not an empty hash

	// GetFeeForMessage (transfer to self, successful)
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				1,
				pubKey,
				pubKey,
			).Build(),
		},
		hash.Value.Blockhash,
		solana.TransactionPayer(pubKey),
	)
	assert.NoError(t, err)

	fee, err := c.GetFeeForMessage(ctx, tx.Message.ToBase64())
	assert.NoError(t, err)
	assert.Equal(t, uint64(5000), fee)

	// get chain ID based on gensis hash
	network, err := c.ChainID(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, mn.StringID("localnet"), network)

	// get account info (also tested inside contract_test)
	res, err := c.GetAccountInfoWithOpts(ctx, solana.PublicKey{}, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentFinalized})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), res.Value.Lamports)
	assert.Equal(t, "NativeLoader1111111111111111111111111111111", res.Value.Owner.String())

	// get block + check for nonzero values
	block, err := c.GetLatestBlock(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, solana.Hash{}, block.Blockhash)
	assert.NotEqual(t, uint64(0), block.ParentSlot)
	assert.NotEqual(t, uint64(0), block.ParentSlot)

	// GetBlock
	// Test fetching a valid block
	block, err = c.GetBlock(ctx, slot0)
	assert.NoError(t, err)
	assert.NotNil(t, block)
	assert.Equal(t, slot0, block.ParentSlot+1)
	assert.NotEqual(t, solana.Hash{}, block.Blockhash)

	// Test fetching a block with an invalid future slot
	futureSlot := slot0 + 1000000
	block, err = c.GetBlock(ctx, futureSlot)
	assert.Error(t, err)
	assert.Nil(t, block)

	// GetBlocksWithLimit
	// Define the limit of blocks to fetch and calculate the start slot
	limit := uint64(10)
	startSlot := slot0 - limit + 1

	// Fetch blocks with limit
	blocksResult, err := c.GetBlocksWithLimit(ctx, startSlot, limit)
	assert.NoError(t, err)
	assert.NotNil(t, blocksResult)

	// Verify that the slots returned are within the expected range
	for _, slot := range *blocksResult {
		assert.GreaterOrEqual(t, slot, startSlot)
		assert.LessOrEqual(t, slot, slot0)
	}
}

func TestClient_Reader_ChainID(t *testing.T) {
	ctx := tests.Context(t)
	genesisHashes := []string{
		DevnetGenesisHash,  // devnet
		TestnetGenesisHash, // testnet
		MainnetGenesisHash, // mainnet
		"GH7ome3EiwEr7tu9JuTh2dpYWBJK3z69Xm1ZE3MEE6JC", // localnet (random)
	}
	networks := []string{"devnet", "testnet", "mainnet", "localnet"}
	hashCounter := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := fmt.Sprintf(`{"jsonrpc":"2.0","result":"%s","id":1}`, genesisHashes[hashCounter])
		hashCounter++
		_, err := w.Write([]byte(out))
		require.NoError(t, err)
	}))
	defer mockServer.Close()

	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	c, err := NewClient(mockServer.URL, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	// get chain ID based on gensis hash
	for _, n := range networks {
		network, err := c.ChainID(ctx)
		assert.NoError(t, err)
		assert.Equal(t, mn.StringID(n), network)
	}
}

func TestClient_Writer_Integration(t *testing.T) {
	url := SetupLocalSolNode(t)
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privKey.PublicKey()
	FundTestAccounts(t, []solana.PublicKey{pubKey}, url)

	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()

	ctx := tests.Context(t)
	c, err := NewClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	// create + sign transaction
	createTx := func(to solana.PublicKey) *solana.Transaction {
		hash, hashErr := c.LatestBlockhash(ctx)
		assert.NoError(t, hashErr)

		tx, txErr := solana.NewTransaction(
			[]solana.Instruction{
				system.NewTransferInstruction(
					1,
					pubKey,
					to,
				).Build(),
			},
			hash.Value.Blockhash,
			solana.TransactionPayer(pubKey),
		)
		assert.NoError(t, txErr)
		_, signErr := tx.Sign(
			func(key solana.PublicKey) *solana.PrivateKey {
				if pubKey.Equals(key) {
					return &privKey
				}
				return nil
			},
		)
		assert.NoError(t, signErr)
		return tx
	}

	// simulate successful transcation
	txSuccess := createTx(pubKey)
	simSuccess, err := c.SimulateTx(ctx, txSuccess, nil)
	assert.NoError(t, err)
	assert.Nil(t, simSuccess.Err)
	assert.Equal(t, 0, len(simSuccess.Accounts)) // default option, no accounts requested

	// simulate successful transcation with custom options
	simCustom, err := c.SimulateTx(ctx, txSuccess, &rpc.SimulateTransactionOpts{
		Commitment: c.commitment,
		Accounts: &rpc.SimulateTransactionAccountsOpts{
			Encoding:  solana.EncodingBase64,
			Addresses: txSuccess.Message.AccountKeys, // request data for accounts in the tx
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, len(txSuccess.Message.AccountKeys), len(simCustom.Accounts)) // data should be returned for the accounts in the tx

	// simulate failed transaction
	txFail := createTx(solana.MustPublicKeyFromBase58("11111111111111111111111111111111"))
	simFail, err := c.SimulateTx(ctx, txFail, nil)
	assert.NoError(t, err)
	assert.NotNil(t, simFail.Err)

	// send successful + failed tx to get tx signatures
	sigSuccess, err := c.SendTx(ctx, txSuccess)
	assert.NoError(t, err)

	sigFail, err := c.SendTx(ctx, txFail)
	assert.NoError(t, err)

	// check signature statuses
	time.Sleep(2 * time.Second) // wait for processing
	statuses, err := c.SignatureStatuses(ctx, []solana.Signature{sigSuccess, sigFail})
	assert.NoError(t, err)

	assert.Nil(t, statuses[0].Err)
	assert.NotNil(t, statuses[1].Err)
}

func TestClient_SendTxDuplicates_Integration(t *testing.T) {
	ctx := tests.Context(t)
	// set up environment
	url := SetupLocalSolNode(t)
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	pubKey := privKey.PublicKey()
	FundTestAccounts(t, []solana.PublicKey{pubKey}, url)

	// create client
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := config.NewDefault()
	c, err := NewClient(url, cfg, requestTimeout, lggr)
	require.NoError(t, err)

	// fetch recent blockhash
	hash, err := c.LatestBlockhash(ctx)
	assert.NoError(t, err)

	initBal, err := c.Balance(ctx, pubKey)
	assert.NoError(t, err)

	// create + sign tx
	// tx sends tokens to self
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				1,
				pubKey,
				pubKey,
			).Build(),
		},
		hash.Value.Blockhash,
		solana.TransactionPayer(pubKey),
	)
	assert.NoError(t, err)
	_, err = tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if pubKey.Equals(key) {
				return &privKey
			}
			return nil
		},
	)
	assert.NoError(t, err)

	// send 5 of the same transcation
	n := 5
	sigs := make([]solana.Signature, n)
	var wg sync.WaitGroup
	wg.Add(5)
	for i := 0; i < n; i++ {
		go func(i int) {
			time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond) // randomly submit txs
			sig, sendErr := c.SendTx(ctx, tx)
			assert.NoError(t, sendErr)
			sigs[i] = sig
			wg.Done()
		}(i)
	}
	wg.Wait()

	// expect one single transaction hash
	for i := 1; i < n; i++ {
		assert.Equal(t, sigs[0], sigs[i])
	}

	// try waiting for tx to execute - reduce flakiness
	require.Eventually(t, func() bool {
		res, statusErr := c.SignatureStatuses(ctx, []solana.Signature{sigs[0]})
		require.NoError(t, statusErr)
		require.Equal(t, 1, len(res))
		if res[0] == nil {
			return false
		}
		return res[0].ConfirmationStatus == rpc.ConfirmationStatusConfirmed
	}, 5*time.Second, 500*time.Millisecond)

	// expect one sender has only sent one tx
	// original balance - current bal = 5000 lamports (tx fee)
	endBal, err := c.Balance(ctx, pubKey)
	assert.NoError(t, err)
	assert.Equal(t, uint64(5_000), initBal-endBal)
}

func TestClientLatency(t *testing.T) {
	c := Client{}
	v := 100
	n := t.Name() + uuid.NewString()
	f := func() {
		done := c.latency(n)
		defer done()
		time.Sleep(time.Duration(v) * time.Millisecond)
	}
	f()
	g, err := monitor.GetClientLatency(n, c.url)
	require.NoError(t, err)
	val := testutil.ToFloat64(g)

	// check within expected range
	assert.GreaterOrEqual(t, val, float64(v))
	assert.LessOrEqual(t, val, float64(v)*1.05)
}
