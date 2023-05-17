package main

import (
	"bytes"
	"encoding/binary"
	"log"

	"blockchainInGo/base58"
)

// IntToHex converts an int64 to a byte array
func IntToHex(num int64) []byte {
	buff := new(bytes.Buffer)
	err := binary.Write(buff, binary.BigEndian, num)
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

func DecodeAddressToPubKeyHash(address string) []byte {
	byteAddress := base58.Decode(address)
	pubKeyHash := byteAddress[1 : len(byteAddress)-addressChecksumLen]

	return pubKeyHash
}
