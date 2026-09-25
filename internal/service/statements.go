package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"time"
	_ "time/tzdata" // statement dates are placed in the user's timezone

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/gen/nagomi/v1/nagomiv1connect"
	"nagomi-core/internal/storage"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/protobuf/encoding/protojson"
)

// pending statements nobody committed are dropped after this long
const pendingStatementTTL = 24 * time.Hour

// ----- interface ---------------------------------------------------------------------------

type StatementService interface {
	Preview(ctx context.Context, userID uuid.UUID, pdfData []byte, fileName string) (*pb.PreviewStatementImportResponse, error)
	Commit(ctx context.Context, userID uuid.UUID, req *pb.CommitStatementImportRequest) (*pb.CommitStatementImportResponse, error)
	List(ctx context.Context, userID uuid.UUID, req *pb.ListStatementsRequest) ([]*pb.Statement, error)
	Get(ctx context.Context, userID uuid.UUID, id int64) (*pb.Statement, []byte, error)
	Delete(ctx context.Context, userID uuid.UUID, id int64, deleteTransactions bool) (int32, error)
	StartCleanup(ctx context.Context)
}

type stmtSvc struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	log     *log.Logger
	parser  nagomiv1connect.StatementParserServiceClient
	store   *storage.Store
	txnSvc  TransactionService
}

func newStmtSvc(
	pool *pgxpool.Pool,
	queries *sqlc.Queries,
	logger *log.Logger,
	parserURL string,
	store *storage.Store,
	txnSvc TransactionService,
) StatementService {
	var parser nagomiv1connect.StatementParserServiceClient
	if parserURL != "" {
		parser = nagomiv1connect.NewStatementParserServiceClient(
			newH2CClient(),
			parserURL,
			connect.WithGRPC(),
			connect.WithSendMaxBytes(32*1024*1024),
			connect.WithReadMaxBytes(32*1024*1024),
		)
	}
	return &stmtSvc{
		pool:    pool,
		queries: queries,
		log:     logger,
		parser:  parser,
		store:   store,
		txnSvc:  txnSvc,
	}
}

// DuplicateStatementError is returned when an uploaded file was already imported.
type DuplicateStatementError struct {
	ExistingID int64
}

func (e *DuplicateStatementError) Error() string {
	return fmt.Sprintf("statement already imported (id %d): %v", e.ExistingID, ErrDuplicate)
}

func (e *DuplicateStatementError) Unwrap() error {
	return ErrDuplicate
}

// ----- methods -----------------------------------------------------------------------------

func (s *stmtSvc) Preview(ctx context.Context, userID uuid.UUID, pdfData []byte, fileName string) (*pb.PreviewStatementImportResponse, error) {
	if s.parser == nil {
		return nil, fmt.Errorf("StatementService.Preview: NAGOMI_STATEMENTS_URL is not set: %w", ErrUnimplemented)
	}

	sum := sha256.Sum256(pdfData)
	fileHash := hex.EncodeToString(sum[:])

	existing, err := s.queries.GetStatementByHash(ctx, sqlc.GetStatementByHashParams{UserID: userID, FileHash: fileHash})
	switch {
	case err == nil && pb.StatementStatus(existing.Status) == pb.StatementStatus_STATEMENT_STATUS_IMPORTED:
		return nil, &DuplicateStatementError{ExistingID: existing.ID}
	case err == nil:
		return s.buildPreview(ctx, userID, existing, nil)
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, wrapErr("StatementService.Preview.GetByHash", err)
	}

	resp, err := s.parser.ParseStatement(ctx, connect.NewRequest(&pb.ParseStatementRequest{PdfData: pdfData}))
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeInvalidArgument {
			return nil, fmt.Errorf("StatementService.Preview: %s: %w", connectErr.Message(), ErrValidation)
		}
		return nil, fmt.Errorf("StatementService.Preview.Parse: %w", err)
	}
	parsed := resp.Msg.GetStatement()

	parsedJSON, err := protojson.Marshal(parsed)
	if err != nil {
		return nil, fmt.Errorf("StatementService.Preview.Marshal: %w", err)
	}

	key := path.Join("statements", userID.String(), uuid.New().String()+".pdf")
	if err := s.store.Put(ctx, key, pdfData, "application/pdf"); err != nil {
		return nil, fmt.Errorf("StatementService.Preview: %w", err)
	}

	currency := parsed.GetCurrency()
	if currency == "" {
		currency = "CAD"
	}

	row, err := s.queries.CreateStatement(ctx, sqlc.CreateStatementParams{
		UserID:              userID,
		Status:              int16(pb.StatementStatus_STATEMENT_STATUS_PENDING),
		FilePath:            key,
		FileHash:            fileHash,
		FileName:            fileName,
		Parser:              parsed.GetParser(),
		Bank:                parsed.GetBank(),
		AccountType:         int16(parsed.GetAccountType()),
		AccountNumber:       parsed.GetAccountNumber(),
		PeriodStart:         dateToUTC(parsed.GetPeriodStart()),
		PeriodEnd:           dateToUTC(parsed.GetPeriodEnd()),
		Currency:            currency,
		OpeningBalanceCents: parsed.OpeningBalanceCents,
		ClosingBalanceCents: parsed.ClosingBalanceCents,
		LineCount:           int32(len(parsed.GetLines())),
		Parsed:              parsedJSON,
	})
	if err != nil {
		if delErr := s.store.Delete(ctx, key); delErr != nil {
			s.log.Warn("failed to clean up orphaned statement blob", "key", key, "error", delErr)
		}
		return nil, wrapErr("StatementService.Preview.Create", err)
	}

	return s.buildPreview(ctx, userID, row, parsed)
}

func (s *stmtSvc) Commit(ctx context.Context, userID uuid.UUID, req *pb.CommitStatementImportRequest) (*pb.CommitStatementImportResponse, error) {
	row, err := s.queries.GetStatement(ctx, sqlc.GetStatementParams{ID: req.GetStatementId(), UserID: userID})
	if err != nil {
		return nil, wrapErr("StatementService.Commit.Get", err)
	}
	stmt := row.Statement
	if pb.StatementStatus(stmt.Status) != pb.StatementStatus_STATEMENT_STATUS_PENDING {
		return nil, fmt.Errorf("StatementService.Commit: statement %d is already imported: %w", stmt.ID, ErrValidation)
	}

	parsed, err := unmarshalParsed(stmt.Parsed)
	if err != nil {
		return nil, fmt.Errorf("StatementService.Commit: %w", err)
	}

	loc := s.userLocation(ctx, userID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("StatementService.Commit.Begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	account, err := resolveStatementAccount(ctx, q, userID, &stmt, req)
	if err != nil {
		return nil, err
	}
	if account.MainCurrency != stmt.Currency {
		return nil, fmt.Errorf(
			"StatementService.Commit: statement is in %s but account %q is in %s: %w",
			stmt.Currency, account.Name, account.MainCurrency, ErrValidation,
		)
	}

	// no-op on accounts shared with the user, which only the owner can alias
	if stmt.AccountNumber != "" {
		if err := q.AddAccountAlias(ctx, sqlc.AddAccountAliasParams{ID: account.ID, UserID: userID, Alias: stmt.AccountNumber}); err != nil {
			return nil, wrapErr("StatementService.Commit.AddAlias", err)
		}
	}

	externalIDs := lineExternalIDs(parsed.GetLines())
	source := int16(pb.TransactionSource_TRANSACTION_SOURCE_STATEMENT)
	notManual := false
	createdIDs := make([]int64, 0, len(parsed.GetLines()))
	var duplicates int32

	for i, line := range parsed.GetLines() {
		created, err := q.CreateTransaction(ctx, sqlc.CreateTransactionParams{
			UserID:              userID,
			AccountID:           account.ID,
			ExternalID:          &externalIDs[i],
			TxDate:              dateIn(line.GetDate(), loc),
			TxAmountCents:       line.GetAmountCents(),
			TxCurrency:          stmt.Currency,
			TxDirection:         int16(line.GetDirection()),
			TxDesc:              &line.Description,
			CategoryManuallySet: &notManual,
			MerchantManuallySet: &notManual,
			Source:              source,
			StatementID:         &stmt.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			duplicates++
			continue
		}
		if err != nil {
			return nil, wrapErr("StatementService.Commit.CreateTransaction", err)
		}
		createdIDs = append(createdIDs, created.ID)
	}

	imported, err := q.MarkStatementImported(ctx, sqlc.MarkStatementImportedParams{ID: stmt.ID, UserID: userID, AccountID: account.ID})
	if err != nil {
		return nil, wrapErr("StatementService.Commit.MarkImported", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("StatementService.Commit: %w", err)
	}

	if err := s.queries.SyncAccountBalances(ctx, account.ID); err != nil {
		s.log.Warn("failed to sync account balances", "account_id", account.ID, "error", err)
	}
	s.txnSvc.ApplyRules(ctx, userID, createdIDs)

	return &pb.CommitStatementImportResponse{
		Statement:      statementToPb(&imported, &account.Name),
		CreatedCount:   int32(len(createdIDs)),
		DuplicateCount: duplicates,
	}, nil
}

func (s *stmtSvc) List(ctx context.Context, userID uuid.UUID, req *pb.ListStatementsRequest) ([]*pb.Statement, error) {
	params := sqlc.ListStatementsParams{UserID: userID, AccountID: req.AccountId}
	if req.Status != nil {
		status := int16(req.GetStatus())
		params.Status = &status
	}

	rows, err := s.queries.ListStatements(ctx, params)
	if err != nil {
		return nil, wrapErr("StatementService.List", err)
	}

	statements := make([]*pb.Statement, len(rows))
	for i := range rows {
		statements[i] = statementToPb(&rows[i].Statement, rows[i].AccountName)
	}
	return statements, nil
}

func (s *stmtSvc) Get(ctx context.Context, userID uuid.UUID, id int64) (*pb.Statement, []byte, error) {
	row, err := s.queries.GetStatement(ctx, sqlc.GetStatementParams{ID: id, UserID: userID})
	if err != nil {
		return nil, nil, wrapErr("StatementService.Get", err)
	}

	pdfData, err := s.store.Get(ctx, row.Statement.FilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("StatementService.Get: %w", err)
	}

	return statementToPb(&row.Statement, row.AccountName), pdfData, nil
}

func (s *stmtSvc) Delete(ctx context.Context, userID uuid.UUID, id int64, deleteTransactions bool) (int32, error) {
	row, err := s.queries.GetStatement(ctx, sqlc.GetStatementParams{ID: id, UserID: userID})
	if err != nil {
		return 0, wrapErr("StatementService.Delete.Get", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("StatementService.Delete.Begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries.WithTx(tx)

	var deleted int64
	if deleteTransactions {
		deleted, err = q.DeleteStatementTransactions(ctx, sqlc.DeleteStatementTransactionsParams{ID: id, UserID: userID})
		if err != nil {
			return 0, wrapErr("StatementService.Delete.Transactions", err)
		}
	}

	filePath, err := q.DeleteStatement(ctx, sqlc.DeleteStatementParams{ID: id, UserID: userID})
	if err != nil {
		return 0, wrapErr("StatementService.Delete", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("StatementService.Delete: %w", err)
	}

	if err := s.store.Delete(ctx, filePath); err != nil {
		s.log.Warn("failed to delete statement blob", "key", filePath, "error", err)
	}
	if deleted > 0 && row.Statement.AccountID != nil {
		if err := s.queries.SyncAccountBalances(ctx, *row.Statement.AccountID); err != nil {
			s.log.Warn("failed to sync account balances", "account_id", *row.Statement.AccountID, "error", err)
		}
	}

	return int32(deleted), nil
}

// StartCleanup drops pending statements that were previewed but never committed.
func (s *stmtSvc) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		paths, err := s.queries.DeleteStalePendingStatements(ctx, time.Now().Add(-pendingStatementTTL))
		if err != nil {
			s.log.Warn("failed to delete stale pending statements", "error", err)
		}
		for _, p := range paths {
			if err := s.store.Delete(ctx, p); err != nil {
				s.log.Warn("failed to delete statement blob", "key", p, "error", err)
			}
		}
		if len(paths) > 0 {
			s.log.Info("dropped stale pending statements", "count", len(paths))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ----- helpers -----------------------------------------------------------------------------

// buildPreview takes parsed when the caller has it already, else reads it off the row.
func (s *stmtSvc) buildPreview(ctx context.Context, userID uuid.UUID, row sqlc.Statement, parsed *pb.ParsedStatement) (*pb.PreviewStatementImportResponse, error) {
	if parsed == nil {
		var err error
		if parsed, err = unmarshalParsed(row.Parsed); err != nil {
			return nil, fmt.Errorf("StatementService.Preview: %w", err)
		}
	}

	resp := &pb.PreviewStatementImportResponse{
		Statement: statementToPb(&row, nil),
		Lines:     parsed.GetLines(),
	}
	if row.AccountNumber == "" {
		return resp, nil
	}

	match, err := s.queries.FindAccountByAlias(ctx, sqlc.FindAccountByAliasParams{UserID: userID, Alias: row.AccountNumber})
	if errors.Is(err, pgx.ErrNoRows) {
		return resp, nil
	}
	if err != nil {
		return nil, wrapErr("StatementService.Preview.FindAccount", err)
	}
	resp.MatchedAccountId = &match.Account.ID

	duplicates, err := s.queries.CountExistingExternalIDs(ctx, sqlc.CountExistingExternalIDsParams{
		AccountID:   match.Account.ID,
		ExternalIds: lineExternalIDs(parsed.GetLines()),
	})
	if err != nil {
		return nil, wrapErr("StatementService.Preview.CountDuplicates", err)
	}
	resp.DuplicateCount = int32(duplicates)

	return resp, nil
}

func resolveStatementAccount(
	ctx context.Context,
	q *sqlc.Queries,
	userID uuid.UUID,
	stmt *sqlc.Statement,
	req *pb.CommitStatementImportRequest,
) (sqlc.Account, error) {
	switch target := req.GetAccount().(type) {
	case *pb.CommitStatementImportRequest_AccountId:
		row, err := q.GetAccount(ctx, sqlc.GetAccountParams{UserID: userID, ID: target.AccountId})
		if err != nil {
			return sqlc.Account{}, wrapErr("StatementService.Commit.GetAccount", err)
		}
		return row.Account, nil

	case *pb.CommitStatementImportRequest_NewAccountName:
		_, err := q.FindAccountByName(ctx, sqlc.FindAccountByNameParams{UserID: userID, Name: target.NewAccountName})
		if err == nil {
			return sqlc.Account{}, fmt.Errorf("StatementService.Commit: an account named %q already exists: %w", target.NewAccountName, ErrValidation)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return sqlc.Account{}, wrapErr("StatementService.Commit.FindAccount", err)
		}

		account, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{
			OwnerID:        userID,
			Name:           target.NewAccountName,
			Bank:           stmt.Bank,
			AccountType:    stmt.AccountType,
			AnchorCurrency: stmt.Currency,
			MainCurrency:   stmt.Currency,
			Colors:         defaultAccountColors,
		})
		if err != nil {
			return sqlc.Account{}, wrapErr("StatementService.Commit.CreateAccount", err)
		}
		return account, nil
	}

	return sqlc.Account{}, fmt.Errorf("StatementService.Commit: account_id or new_account_name is required: %w", ErrValidation)
}

func (s *stmtSvc) userLocation(ctx context.Context, userID uuid.UUID) *time.Location {
	user, err := s.queries.GetUser(ctx, userID)
	if err != nil {
		s.log.Warn("failed to load user timezone, using UTC", "user_id", userID, "error", err)
		return time.UTC
	}
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		s.log.Warn("unknown user timezone, using UTC", "timezone", user.Timezone, "error", err)
		return time.UTC
	}
	return loc
}

// lineExternalIDs derives a stable id per line, so importing the same statement
// again, or one that overlaps it, adds nothing. identical lines (two coffees on
// the same day) are told apart by the order they appear in.
func lineExternalIDs(lines []*pb.ParsedStatementLine) []string {
	seen := make(map[string]int, len(lines))
	ids := make([]string, len(lines))

	for i, line := range lines {
		d := line.GetDate()
		key := fmt.Sprintf("%04d-%02d-%02d|%d|%d|%s", d.GetYear(), d.GetMonth(), d.GetDay(), line.GetAmountCents(), line.GetDirection(), line.GetDescription())
		occurrence := seen[key]
		seen[key]++

		sum := sha256.Sum256(fmt.Appendf(nil, "%s|%d", key, occurrence))
		ids[i] = "stmt:" + hex.EncodeToString(sum[:12])
	}
	return ids
}

func unmarshalParsed(data []byte) (*pb.ParsedStatement, error) {
	var parsed pb.ParsedStatement
	if err := protojson.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode stored statement: %w", err)
	}
	return &parsed, nil
}

func dateToUTC(d *date.Date) time.Time {
	return dateIn(d, time.UTC)
}

// dateIn is midnight of d in loc.
func dateIn(d *date.Date, loc *time.Location) time.Time {
	return time.Date(int(d.GetYear()), time.Month(d.GetMonth()), int(d.GetDay()), 0, 0, 0, 0, loc)
}

func statementToPb(s *sqlc.Statement, accountName *string) *pb.Statement {
	return &pb.Statement{
		Id:                  s.ID,
		Status:              pb.StatementStatus(s.Status),
		AccountId:           s.AccountID,
		AccountName:         accountName,
		FileName:            s.FileName,
		Parser:              s.Parser,
		Bank:                s.Bank,
		AccountType:         pb.AccountType(s.AccountType),
		AccountNumber:       s.AccountNumber,
		PeriodStart:         timeToDate(s.PeriodStart),
		PeriodEnd:           timeToDate(s.PeriodEnd),
		Currency:            s.Currency,
		OpeningBalanceCents: s.OpeningBalanceCents,
		ClosingBalanceCents: s.ClosingBalanceCents,
		LineCount:           s.LineCount,
		CreatedAt:           toProtoTimestamp(&s.CreatedAt),
		ImportedAt:          toProtoTimestamp(s.ImportedAt),
	}
}
