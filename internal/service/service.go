package service

import (
	"fmt"

	"nagomi-core/internal/config"
	"nagomi-core/internal/crypto"
	"nagomi-core/internal/db"
	"nagomi-core/internal/exchange"
	"nagomi-core/internal/storage"

	"github.com/charmbracelet/log"
)

type Services struct {
	Transactions TransactionService
	Categories   CategoryService
	Rules        RuleService
	Accounts     AccountService
	Dashboard    DashboardService
	Users        UserService
	Receipts     ReceiptService
	Connector    ConnectorService
	Connections  ConnectionService
}

func New(database *db.DB, logger *log.Logger, cfg *config.Config) (*Services, error) {
	queries := database.Queries
	catSvc := newCatSvc(queries, logger.WithPrefix("cat"))
	ruleSvc := newCatRuleSvc(queries, logger.WithPrefix("rules"))
	exchangeClient := exchange.NewClient(cfg.ExchangeAPIURL)

	store := storage.New(storage.S3Config{
		Endpoint:  cfg.S3Endpoint,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Region:    cfg.S3Region,
	})

	cipher, err := crypto.NewFromHex(cfg.CredentialsKey)
	if err != nil {
		return nil, fmt.Errorf("init credentials cipher: %w", err)
	}

	return &Services{
		Transactions: newTxnSvc(queries, logger.WithPrefix("txn"), catSvc, ruleSvc, exchangeClient),
		Categories:   catSvc,
		Rules:        ruleSvc,
		Accounts:     newAcctSvc(queries, logger.WithPrefix("acct")),
		Dashboard:    newDashSvc(queries, exchangeClient),
		Users:        newUserSvc(queries, logger.WithPrefix("user")),
		Receipts:     newRcptSvc(queries, logger.WithPrefix("rcpt"), cfg.NagomiReceiptsURL, store),
		Connector:    newConnSvc(queries, cipher, logger.WithPrefix("connector")),
		Connections:  newConnectionSvc(queries, cipher),
	}, nil
}
