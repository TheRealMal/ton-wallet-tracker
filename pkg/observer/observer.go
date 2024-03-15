package observer

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
)

type Observer struct {
	Client       *liteclient.ConnectionPool
	TonAPIClient ton.APIClientWrapped
	Context      context.Context
	Telegram     *tgbotapi.BotAPI
	ChatIDs      []int64
}

func InitObserver(telegramToken string, chatIDs []int64) *Observer {
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

	bot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		return nil
	}
	obs.Telegram = bot
	obs.ChatIDs = chatIDs
	return obs
}

func (observer *Observer) Observe(target string) {
	log.Println("fetching and checking proofs since config init block, it may take near a minute...")
	master, err := observer.TonAPIClient.CurrentMasterchainInfo(context.Background()) // we fetch block just to trigger chain proof check
	if err != nil {
		log.Fatalln("get masterchain info err: ", err.Error())
		return
	}
	log.Println("master proof checks are completed successfully, now communication is 100% safe!")
	treasuryAddress := address.MustParseAddr(target)
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
		txt, txHex := parseTX(tx)
		fmt.Println(txt)
		observer.SendWebhook(txt, txHex)
		lastProcessedLT = tx.LT
	}
	log.Println("something went wrong, transaction listening unexpectedly finished")
}

func (observer *Observer) ListTXs(target string) {
	// we need fresh block info to run get methods
	b, err := observer.TonAPIClient.CurrentMasterchainInfo(observer.Context)
	if err != nil {
		log.Fatalln("get block err:", err.Error())
		return
	}

	addr := address.MustParseAddr(target)

	// we use WaitForBlock to make sure block is ready,
	// it is optional but escapes us from liteserver block not ready errors
	res, err := observer.TonAPIClient.WaitForBlock(b.SeqNo).GetAccount(observer.Context, b, addr)
	if err != nil {
		log.Fatalln("get account err:", err.Error())
		return
	}

	// take last tx info from account info
	lastHash := res.LastTxHash
	lastLt := res.LastTxLT

	fmt.Printf("\nTransactions:\n")
	// load transactions in batches with size 15
	list, err := observer.TonAPIClient.ListTransactions(observer.Context, addr, 15, lastLt, lastHash)
	if err != nil {
		log.Printf("send err: %s", err.Error())
		return
	}

	// reverse list to show the newest first
	sort.Slice(list, func(i, j int) bool {
		return list[i].LT > list[j].LT
	})

	for _, tx := range list {
		txt, _ := parseTX(tx)
		fmt.Println(txt)
		// observer.SendWebhook(txt, txHex)
	}
}

func parseTX(t *tlb.Transaction) (string, string) {
	result := strings.Builder{}
	var destinations []string
	in, out := new(big.Int), new(big.Int)
	if t.IO.Out != nil {
		listOut, err := t.IO.Out.ToSlice()
		if err != nil {
			return "\nOUT MESSAGES NOT PARSED DUE TO ERR: " + err.Error(), ""
		}

		for _, m := range listOut {
			destinations = append(destinations, m.Msg.DestAddr().String())
			if m.MsgType == tlb.MsgTypeInternal {
				out.Add(out, m.AsInternal().Amount.Nano())
			}
		}
	}
	if t.IO.In != nil {
		if t.IO.In.MsgType == tlb.MsgTypeInternal {
			in = t.IO.In.AsInternal().Amount.Nano()
		}
	}
	if in.Cmp(big.NewInt(0)) != 0 {
		intTx := t.IO.In.AsInternal()
		result.WriteString("*TOKEN SELL*\nAmount: `")
		result.WriteString(tlb.FromNanoTON(in).String())
		result.WriteString(" TON`\nFrom: `")
		result.WriteString(intTx.SrcAddr.String())
		result.WriteString("`\n")
	}
	if out.Cmp(big.NewInt(0)) != 0 {
		result.WriteString("*TOKEN BUY*\nAmount: `")
		result.WriteString(tlb.FromNanoTON(out).String())
		result.WriteString(" TON`\nTo: ")
		for _, s := range destinations {
			result.WriteString("`")
			result.WriteString(s)
			result.WriteString("`\n")
		}
	}
	return result.String(), hex.EncodeToString(t.Hash)
}

func (observer *Observer) SendWebhook(text string, txHex string) {
	for _, chatID := range observer.ChatIDs {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = tgbotapi.ModeMarkdownV2
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("VIEW TX", "https://tonviewer.com/transaction/"+txHex),
			),
		)
		msg.DisableWebPagePreview = true
		observer.Telegram.Send(msg)
	}
}
