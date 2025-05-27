# Arbitrum Sequencer Feed Client

This project implements a Go client that connects to the Arbitrum Sequencer Feed, reads and decodes L2 transaction messages, including both single transactions and nested batches.

## 📦 Features

- Connects to `wss://arb1.arbitrum.io/feed`
- Reads and decodes messages of kinds:
  - `SignedTx` — single signed L2 transaction
  - `Batch` — nested batches of transactions (recursive parsing)
- Logs parsed transaction details:
  - Sender (`from`)
  - Recipient (`to`)
  - Value (`value`)
  - Nonce
  - Gas and GasPrice
  - Chain ID

## 🚀 Getting Started

### Requirements

- Go 1.20+
- Dependencies:
  - [`go-ethereum`](https://github.com/ethereum/go-ethereum)
  - [`gorilla/websocket`](https://github.com/gorilla/websocket)

### Installation

```bash
git clone https://github.com/DevWaste/arbitrum-feed-client.git
cd arbitrum-feed-client
go mod tidy
go run main.go
