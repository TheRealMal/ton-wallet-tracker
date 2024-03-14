package observer

import (
	"context"
	"log"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
)

type Observer struct {
	Client       *liteclient.ConnectionPool
	TonAPIClient ton.APIClientWrapped
	Context      context.Context
}

func InitObserver() *Observer {
	obs := &Observer{
		Client: liteclient.NewConnectionPool(),
	}
	cfg, err := liteclient.GetConfigFromUrl(context.Background(), "https://ton.org/global.config.json")
	if err != nil {
		log.Fatalln("get config err: ", err.Error())
		return nil
	}
	err = obs.Client.AddConnectionsFromConfig(context.Background(), cfg)
	if err != nil {
		log.Fatalln("connection err: ", err.Error())
		return nil
	}
	obs.TonAPIClient = ton.NewAPIClient(obs.Client, ton.ProofCheckPolicyFast).WithRetry()
	obs.TonAPIClient.SetTrustedBlockFromConfig(cfg)
	obs.Context = obs.Client.StickyContext(context.Background())
	return obs
}

func (observer *Observer) Observe() {
	log.Println("fetching and checking proofs since config init block, it may take near a minute...")
	master, err := observer.TonAPIClient.CurrentMasterchainInfo(context.Background()) // we fetch block just to trigger chain proof check
	if err != nil {
		log.Fatalln("get masterchain info err: ", err.Error())
		return
	}
	log.Println("master proof checks are completed successfully, now communication is 100% safe!")
	treasuryAddress := address.MustParseAddr("EQCXwWAyDG_IhRh6CzPSetvgGecywZBU3YNCawmz03Uk25RG")
	acc, err := observer.TonAPIClient.GetAccount(context.Background(), master, treasuryAddress)
	if err != nil {
		log.Fatalln("get masterchain info err: ", err.Error())
		return
	}

	// Cursor of processed transaction, save it to your db
	// We start from last transaction, will not process transactions older than we started from.
	// After each processed transaction, save lt to your db, to continue after restart
	lastProcessedLT := acc.LastTxLT
	transactions := make(chan *tlb.Transaction)
	go observer.TonAPIClient.SubscribeOnTransactions(context.Background(), treasuryAddress, lastProcessedLT, transactions)

	log.Println("waiting for transfers...")
	for tx := range transactions {
		log.Println(tx.String())
		lastProcessedLT = tx.LT
	}
	log.Println("something went wrong, transaction listening unexpectedly finished")
}
