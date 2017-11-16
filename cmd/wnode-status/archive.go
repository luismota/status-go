package main

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"time"

	"fmt"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/p2p/discover"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
)

var nodeid *ecdsa.PrivateKey

func getNodeID(shh *whisper.Whisper) *ecdsa.PrivateKey {
	if nodeid != nil {
		return nodeid
	}

	tmpID, err := shh.NewKeyPair()
	if err != nil {
		utils.Fatalf("Failed to generate a new key pair: %s", err)
	}

	nodeid, err = shh.GetPrivateKey(tmpID)
	if err != nil {
		utils.Fatalf("Failed to retrieve a new key pair: %s", err)
	}

	return nodeid
}

func requestExpiredMessagesLoop(shh *whisper.Whisper, topic, mailServerEnode, password string, timeLow, timeUpp uint32, closeCh chan struct{}) error {
	var key, mailServerPeerID []byte
	var xt, empty whisper.TopicType

	keyID, err := shh.AddSymKeyFromPassword(password)
	if err != nil {
		return fmt.Errorf("Failed to create symmetric key for mail request: %s", err)
	}
	key, err = shh.GetSymKey(keyID)
	if err != nil {
		return fmt.Errorf("Failed to save symmetric key for mail request: %s", err)
	}

	mailServerPeerID, err = extractIdFromEnode(mailServerEnode)
	if err != nil {
		return err
	}

	err = shh.AllowP2PMessagesFromPeer(mailServerPeerID)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-closeCh:
			return nil
		case <-ticker.C:
			if len(topic) >= whisper.TopicLength*2 {
				x, err := hex.DecodeString(topic)
				if err != nil {
					return fmt.Errorf("Failed to parse the topic: %s", err)
				}
				xt = whisper.BytesToTopic(x)
			}
			if timeUpp == 0 {
				timeUpp = 0xFFFFFFFF
			}

			data := make([]byte, 8+whisper.TopicLength)
			binary.BigEndian.PutUint32(data, timeLow)
			binary.BigEndian.PutUint32(data[4:], timeUpp)
			copy(data[8:], xt[:])
			if xt == empty {
				data = data[:8]
			}

			var params whisper.MessageParams
			params.PoW = 1
			params.Payload = data
			params.KeySym = key
			params.Src = getNodeID(shh)
			params.WorkTime = 5

			msg, err := whisper.NewSentMessage(&params)
			if err != nil {
				return fmt.Errorf("failed to create new message: %s", err)
			}

			env, err := msg.Wrap(&params)
			if err != nil {
				return fmt.Errorf("Wrap failed: %s", err)
			}

			err = shh.RequestHistoricMessages(mailServerPeerID, env)
			if err != nil {
				return fmt.Errorf("Failed to send P2P message: %s", err)
			}
		}
	}
}

func extractIdFromEnode(s string) ([]byte, error) {
	n, err := discover.ParseNode(s)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse enode: %s", err)
	}
	return n.ID[:], nil
}