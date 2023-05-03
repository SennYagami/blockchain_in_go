rm blockchain.db 
go build
sudo ./blockchainInGo createblockchain -address senn
sudo chmod  777 ./blockchain.db 
sudo ./blockchainInGo getbalance -address senn
sudo ./blockchainInGo send -from senn -to jay -amount 6
sudo ./blockchainInGo getbalance -address senn
sudo ./blockchainInGo getbalance -address jay
sudo ./blockchainInGo send -from senn -to vitalik -amount 3
sudo ./blockchainInGo getbalance -address senn
sudo ./blockchainInGo getbalance -address vitalik
