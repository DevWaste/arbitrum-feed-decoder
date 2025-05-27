package arbitrum

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/gorilla/websocket"
)

const (
	MaxL2MessageSize = 256 * 1024
	MaxBatchDepth    = 16
	WebSocketVersion = "13"
)

type L2MessageKind int

const (
	UnsignedUserTx L2MessageKind = iota
	ContractTx
	NonMutatingCall
	Batch
	SignedTx
	_
	Heartbeat // deprecated
	SignedCompressedTx
)

type DecodedMsg struct {
	Batch       []*types.Transaction
	SignedTx    *types.Transaction
	MessageKind L2MessageKind
}

type Root struct {
	Version  uint8                  `json:"version"`
	Messages []BroadcastFeedMessage `json:"messages"`
}

type BroadcastFeedMessage struct {
	SequenceNumber uint64              `json:"sequenceNumber"`
	Message        MessageWithMetadata `json:"message"`
	Signature      json.RawMessage     `json:"signature"`
}

type MessageWithMetadata struct {
	Message             L1IncomingMessageHeader `json:"message"`
	DelayedMessagesRead uint64                  `json:"delayedMessagesRead"`
}

type L1IncomingMessageHeader struct {
	Header Header `json:"header"`
	L2Msg  string `json:"l2Msg"`
}

type Header struct {
	Kind        uint8           `json:"kind"`
	Sender      string          `json:"sender"`
	BlockNumber uint64          `json:"blockNumber"`
	Timestamp   uint64          `json:"timestamp"`
	RequestID   json.RawMessage `json:"requestId"`
	BaseFeeL1   json.RawMessage `json:"baseFeeL1"`
}

type RelayClient struct {
	conn         *websocket.Conn
	sender       chan<- *DecodedMsg
	connectionID uint32
	chainID      *big.Int
}

func NewRelayClient(wsURL *url.URL, chainID *big.Int, sender chan<- *DecodedMsg, connectionID uint32) (*RelayClient, error) {
	header := http.Header{
		"Arbitrum-Feed-Client-Version":       []string{"2"},
		"Arbitrum-Requested-Sequence-number": []string{"0"},
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err != nil {
		return nil, fmt.Errorf("websocket connection failed: %v", err)
	}

	return &RelayClient{
		conn:         conn,
		sender:       sender,
		connectionID: connectionID,
		chainID:      chainID,
	}, nil
}

func (rc *RelayClient) Run(ctx context.Context) error {
	defer rc.conn.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, message, err := rc.conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				return err
			}

			var root Root
			if err := json.Unmarshal(message, &root); err != nil {
				log.Printf("JSON decode error: %v", err)
				continue
			}

			for _, msg := range root.Messages {
				decoded, err := decodeMessage(msg.Message.Message)
				if err != nil {
					log.Printf("Decoding error: %v", err)
					continue
				}

				select {
				case rc.sender <- decoded:
				case <-ctx.Done():
					return nil
				}
			}
		}
	}
}

func decodeMessage(header L1IncomingMessageHeader) (*DecodedMsg, error) {
	if len(header.L2Msg) > MaxL2MessageSize {
		return nil, errors.New("message size exceeds maximum")
	}

	decoded, err := base64.StdEncoding.DecodeString(header.L2Msg)
	if err != nil {
		return nil, fmt.Errorf("base64 decode error: %v", err)
	}

	if len(decoded) == 0 {
		return nil, errors.New("empty L2 message")
	}

	messageKind := L2MessageKind(decoded[0])
	data := decoded[1:]

	switch messageKind {
	case Batch:
		txs, err := parseBatchTransactions(data)
		if err != nil {
			return nil, err
		}
		return &DecodedMsg{Batch: txs, MessageKind: Batch}, nil
	case SignedTx:
		tx, err := parseSignedTransaction(data)
		if err != nil {
			return nil, err
		}
		return &DecodedMsg{SignedTx: tx, MessageKind: SignedTx}, nil
	default:
		return nil, fmt.Errorf("unsupported message kind: %d", messageKind)
	}
}

func parseBatchTransactions(data []byte) ([]*types.Transaction, error) {
	return parseBatchTransactionsDepth(data, 0)
}

func parseBatchTransactionsDepth(data []byte, depth int) ([]*types.Transaction, error) {
	if depth > MaxBatchDepth {
		return nil, fmt.Errorf("nested batch depth too deep (limit %d)", MaxBatchDepth)
	}

	var txs []*types.Transaction
	offset := 0

	for offset <= len(data)-8 {
		if offset+8 > len(data) {
			return nil, fmt.Errorf("incomplete size prefix at offset %d", offset)
		}

		size := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		end := offset + int(size)
		if end > len(data) {
			return nil, fmt.Errorf("declared size %d exceeds data at offset %d", size, offset)
		}

		msg := data[offset:end]
		offset = end

		if len(msg) < 2 {
			log.Printf("⚠️ Skipping message: too short (%d bytes)", len(msg))
			continue
		}

		messageKind := L2MessageKind(msg[0])
		payload := msg[1:]

		switch messageKind {
		case Batch:
			nestedTxs, err := parseBatchTransactionsDepth(payload, depth+1)
			if err != nil {
				return nil, fmt.Errorf("nested batch decode error: %w", err)
			}
			txs = append(txs, nestedTxs...)

		case SignedTx:
			tx, err := parseSignedTransaction(payload)
			if err != nil {
				log.Printf("⚠️ Skipping invalid signed tx: %v", err)
				log.Printf("    Payload (hex): %s", fmt.Sprintf("%x", payload))
				log.Printf("    Payload (len): %d bytes", len(payload))
				continue
			}
			txs = append(txs, tx)

		default:
			log.Printf("⚠️ Unsupported message kind in batch: %d", messageKind)
			continue
		}
	}

	return txs, nil
}

func parseSignedTransaction(data []byte) (*types.Transaction, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(data); err != nil {
		return nil, fmt.Errorf("binary decode error: %v", err)
	}
	return &tx, nil
}
