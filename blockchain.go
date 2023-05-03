package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/boltdb/bolt"
)

const dbFile = "blockchain.db"
const blocksBucket = "blocks"
const genesisCoinbaseData = "The Times 03/Jan/2009 Chancellor on brink of second bailout for banks"

// Blockchain keeps a sequence of blocks
type Blockchain struct {
	tip []byte //the head block's hash
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
	db, err := bolt.Open(dbFile, 0600, nil)
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
	var tip []byte

	db, err := bolt.Open(dbFile, 0600, nil)

	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))

		if b == nil {
			fmt.Println("No existing blockchain found. Creating a new one...")

			cbtx := NewCoinbaseTX(address, genesisCoinbaseData)
			genesis := NewGenesisBlock(cbtx)

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

		} else {
			tip = b.Get([]byte("l"))
		}

		return nil
	},
	)

	if err != nil {
		log.Panic(err)
	}

	bc := Blockchain{tip, db}

	return &bc

}

// saves provided data as a block in the blockchain
func (bc *Blockchain) MineBlock(transactions []*Transaction) {
	var lastHash []byte

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
	spentTXOs := make(map[string][]int)
	unspentTXOs := make(map[string][]OutIndexAndData)

	bci := bc.Iterator()

	for {
		block := bci.Next()

		for _, tx := range block.Transactions {

			txId := hex.EncodeToString(tx.ID)

		Work:
			for index := range tx.Vout {
				if !tx.Vout[index].CanBeUnlockedWith(address) {
					continue
				}

				for _, spentTXOIndex := range spentTXOs[txId] {
					// this out has been used, so jump to the next out check
					if spentTXOIndex == index {
						continue Work
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
