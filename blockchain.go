package main

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/boltdb/bolt"
)

const (
	dbFile              = "blockchain.db"
	blocksBucket        = "blocks"
	genesisCoinbaseData = "The Times 03/Jan/2009 Chancellor on brink of second bailout for banks"
)

// Blockchain keeps a sequence of blocks
type Blockchain struct {
	tip []byte // the head block's hash
	db  *bolt.DB
}

// used to iterate over blockchain blocks
type BlockchainIterator struct {
	currentHash []byte
	db          *bolt.DB
}

type OutIndexAndData struct {
	i   int
	out *TXOutput
}

func dbExists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}

	return true
}

// NewBlockchain creates a new Blockchain with genesis Block
func NewBlockchain(address string) *Blockchain {
	if dbExists() == false {
		fmt.Println("No existing blockchain found. Create one first.")
		os.Exit(1)
	}

	var tip []byte
	db, err := bolt.Open(dbFile, 0o600, nil)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		tip = b.Get([]byte("l"))

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	bc := Blockchain{tip, db}

	return &bc
}

// creates a new Blockchain with genesis Block
func CreateBlockchain(address string) *Blockchain {
	if dbExists() {
		fmt.Println("Blockchain already exists.")
		os.Exit(1)
	}

	var tip []byte

	cbtx := NewCoinbaseTX(address, genesisCoinbaseData)
	genesis := NewGenesisBlock(cbtx)

	db, err := bolt.Open(dbFile, 0o600, nil)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte(blocksBucket))
		if err != nil {
			log.Panic(err)
		}

		err = b.Put(genesis.Hash, genesis.Serialize())
		if err != nil {
			log.Panic(err)
		}

		err = b.Put([]byte("l"), genesis.Hash)
		if err != nil {
			log.Panic(err)
		}

		tip = genesis.Hash

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	bc := Blockchain{tip, db}

	return &bc
}

// saves provided data as a block in the blockchain
func (bc *Blockchain) MineBlock(transactions []*Transaction) {
	var lastHash []byte

	for _, tx := range transactions {
		if bc.VerifyTransaction(tx) != true {
			log.Panic("ERROR: Invalid transaction")
		}
	}

	err := bc.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		lastHash = b.Get([]byte("l"))

		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	newBlock := NewBlock(transactions, lastHash)

	err = bc.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		err := b.Put(newBlock.Hash, newBlock.Serialize())
		if err != nil {
			log.Panic(err)
		}

		err = b.Put([]byte("l"), newBlock.Hash)
		if err != nil {
			log.Panic(err)
		}

		bc.tip = newBlock.Hash

		return nil
	})
	if err != nil {
		log.Panic(err)
	}
}

func (bc *Blockchain) Iterator() *BlockchainIterator {
	bci := &BlockchainIterator{bc.tip, bc.db}

	return bci
}

// Next returns next block starting from the tip
func (i *BlockchainIterator) Next() *Block {
	var block *Block

	err := i.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		encodedBlock := b.Get(i.currentHash)

		block = DeserializeBlock(encodedBlock)
		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	i.currentHash = block.PrevBlockHash
	return block
}

func (bc *Blockchain) FindUTXOInTransactions(address string) map[string][]OutIndexAndData {
	pubKeyHash := DecodeAddressToPubKeyHash(address)

	spentTXOs := make(map[string][]int)
	unspentTXOs := make(map[string][]OutIndexAndData)

	bci := bc.Iterator()

	for {
		block := bci.Next()

		for _, tx := range block.Transactions {

			txId := hex.EncodeToString(tx.ID)

		Outputs:
			for index := range tx.Vout {
				if !tx.Vout[index].IsLockedWithKey(pubKeyHash) {
					continue
				}

				for _, spentTXOIndex := range spentTXOs[txId] {
					// this out has been used, so jump to the next out check
					if spentTXOIndex == index {
						continue Outputs
					}
				}

				unspentTXOs[txId] = append(unspentTXOs[txId], OutIndexAndData{index, &tx.Vout[index]})
			}

			if !tx.IsCoinbase() {
				for _, txIn := range tx.Vin {
					vinTxId := hex.EncodeToString(txIn.Txid)
					spentTXOs[vinTxId] = append(spentTXOs[vinTxId], txIn.Vout)
				}
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}

	}
	return unspentTXOs
}

func (bc *Blockchain) FindUTXO(address string) []TXOutput {
	UtxoInTransactions := bc.FindUTXOInTransactions(address)

	var UtxoLs []TXOutput

	for _, outIndexAndData := range UtxoInTransactions {
		for _, out := range outIndexAndData {
			UtxoLs = append(UtxoLs, *out.out)
		}
	}

	return UtxoLs
}

// FindSpendableOutputs finds and returns unspent outputs to reference in inputs
func (bc *Blockchain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {
	UtxoInTransaction := bc.FindUTXOInTransactions(address)
	unspentOutputs := make(map[string][]int)

	var accumulated int
Work:
	for txId, outIndexAndDataLs := range UtxoInTransaction {
		for _, outIndexAndData := range outIndexAndDataLs {
			unspentOutputs[txId] = append(unspentOutputs[txId], outIndexAndData.i)
			accumulated += outIndexAndData.out.Value
			if accumulated >= amount {
				break Work
			}
		}
	}

	return accumulated, unspentOutputs
}

func (bc *Blockchain) FindTransaction(ID []byte) (Transaction, error) {
	bci := bc.Iterator()

	for {
		block := bci.Next()

		for _, tx := range block.Transactions {
			if bytes.Compare(tx.ID, ID) == 0 {
				return *tx, nil
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return Transaction{}, errors.New("Transaction is not found")
}

func (bc *Blockchain) SignTransaction(tx *Transaction, privKey ecdsa.PrivateKey) {
	prevTXs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTX, err := bc.FindTransaction(vin.Txid)
		if err != nil {
			log.Panic(err)
		}
		prevTXs[hex.EncodeToString(prevTX.ID)] = prevTX
	}

	tx.Sign(privKey, prevTXs)
}

func (bc *Blockchain) VerifyTransaction(tx *Transaction) bool {
	prevTxs := make(map[string]Transaction)

	for _, vin := range tx.Vin {
		prevTx, err := bc.FindTransaction(vin.Txid)
		if err != nil {
			log.Panic(err)
		}

		prevTxs[hex.EncodeToString(prevTx.ID)] = prevTx
	}

	return tx.Verify(prevTxs)
}
