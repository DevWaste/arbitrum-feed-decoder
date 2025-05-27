package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"net/url"

	"sequencerfeed/arbitrum"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func main() {
	u, _ := url.Parse("wss://arb1.arbitrum.io/feed")
	chainID := big.NewInt(42161)
	msgChan := make(chan *arbitrum.DecodedMsg)

	client, err := arbitrum.NewRelayClient(u, chainID, msgChan, 1)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for msg := range msgChan {
			processMessage(msg)
		}
	}()

	if err := client.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func processMessage(msg *arbitrum.DecodedMsg) {
	switch msg.MessageKind {
	case arbitrum.Batch:
		log.Printf("🟡 Batch message received: %d transactions", len(msg.Batch))
		for i, tx := range msg.Batch {
			logTx(tx, i)
		}
	case arbitrum.SignedTx:
		if msg.SignedTx != nil {
			logTx(msg.SignedTx, -1)
		} else {
			log.Printf("⚠️ SignedTx message was nil")
		}
	default:
		log.Printf("🔴 Unsupported message kind: %d", msg.MessageKind)
	}
}

func logTx(tx *types.Transaction, index int) {
	msgPrefix := "🟢"
	if index >= 0 {
		msgPrefix = fmt.Sprintf("  ↪️ [%d]", index)
	}

	chainID := big.NewInt(42161)

	var from common.Address
	signer := types.LatestSignerForChainID(chainID)
	if signer == nil {
		signer = types.HomesteadSigner{}
	}
	from, err := types.Sender(signer, tx)
	if err != nil {
		log.Printf("%s   ⚠️ Failed to recover sender: %v", msgPrefix, err)
		from = common.Address{}
	}

	to := tx.To()
	toAddr := "<contract creation>"
	if to != nil {
		toAddr = to.Hex()
	}

	log.Printf("%s tx: %s", msgPrefix, tx.Hash().Hex())
	log.Printf("%s   From:    %s", msgPrefix, from.Hex())
	log.Printf("%s   To:      %s", msgPrefix, toAddr)
	log.Printf("%s   Value:   %s wei", msgPrefix, tx.Value().String())
	log.Printf("%s   Nonce:   %d", msgPrefix, tx.Nonce())
	log.Printf("%s   Gas:     %d @ %s wei", msgPrefix, tx.Gas(), tx.GasPrice().String())
	log.Printf("%s   ChainID: %s", msgPrefix, chainID.String())
}
