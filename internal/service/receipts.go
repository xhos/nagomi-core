package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"
	"nagomi-core/internal/gen/nagomi/v1/nagomiv1connect"
	"nagomi-core/internal/storage"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/google/uuid"
)

// ----- interface ---------------------------------------------------------------------------

type ReceiptService interface {
	Upload(ctx context.Context, userID uuid.UUID, imageData []byte, contentType string) (*pb.Receipt, error)
	Create(ctx context.Context, userID uuid.UUID, req *pb.CreateReceiptRequest) (*pb.Receipt, error)
	Get(ctx context.Context, userID uuid.UUID, id int64) (*pb.Receipt, []*pb.ReceiptLinkCandidate, []byte, error)
	List(ctx context.Context, userID uuid.UUID, req *pb.ListReceiptsRequest) ([]*pb.Receipt, int64, error)
	Update(ctx context.Context, userID uuid.UUID, id int64, req *pb.UpdateReceiptRequest) (*pb.Receipt, error)
	Delete(ctx context.Context, userID uuid.UUID, id int64) error
	RetryParsing(ctx context.Context, userID uuid.UUID, id int64) (*pb.Receipt, error)
	StartWorker(ctx context.Context)
}

type rcptSvc struct {
	queries   *sqlc.Queries
	log       *log.Logger
	ocrClient nagomiv1connect.ReceiptOCRServiceClient
	store     *storage.Store
}

func newRcptSvc(queries *sqlc.Queries, logger *log.Logger, ocrURL string, store *storage.Store) ReceiptService {
	var ocrClient nagomiv1connect.ReceiptOCRServiceClient
	if ocrURL != "" {
		ocrClient = nagomiv1connect.NewReceiptOCRServiceClient(
			newH2CClient(),
			ocrURL,
			connect.WithGRPC(),
			connect.WithSendMaxBytes(16*1024*1024),
			connect.WithReadMaxBytes(16*1024*1024),
		)
	}
	return &rcptSvc{
		queries:   queries,
		log:       logger,
		ocrClient: ocrClient,
		store:     store,
	}
}

// ----- methods -----------------------------------------------------------------------------

func (s *rcptSvc) Upload(ctx context.Context, userID uuid.UUID, imageData []byte, contentType string) (*pb.Receipt, error) {
	if _, ok := contentTypeToExt[contentType]; !ok {
		return nil, fmt.Errorf("ReceiptService.Upload: unsupported content type %q: %w", contentType, ErrValidation)
	}

	// extract EXIF date before resize (resize strips metadata)
	imageTakenAt := extractEXIFDate(imageData)

	imageData, contentType = resizeImage(imageData, contentType)
	ext := contentTypeToExt[contentType]

	sum := sha256.Sum256(imageData)
	imageHash := hex.EncodeToString(sum[:])

	if existing, err := s.queries.GetReceiptByImageHash(ctx, sqlc.GetReceiptByImageHashParams{
		UserID:    userID,
		ImageHash: imageHash,
	}); err == nil {
		if pb.ReceiptStatus(existing.Status) == pb.ReceiptStatus_RECEIPT_STATUS_FAILED {
			// Previous parse failed — reset to pending so the worker retries it.
			updated, resetErr := s.queries.ResetReceiptForRetry(ctx, sqlc.ResetReceiptForRetryParams{
				ID:     existing.ID,
				UserID: userID,
			})
			if resetErr != nil {
				return nil, wrapErr("ReceiptService.Upload.Reset", resetErr)
			}
			return receiptToPb(&updated, nil), nil
		}
		return nil, &DuplicateReceiptError{ExistingID: existing.ID}
	}

	key := path.Join("receipts", userID.String(), uuid.New().String()+"."+ext)

	if err := s.store.Put(ctx, key, imageData, contentType); err != nil {
		return nil, fmt.Errorf("ReceiptService.Upload: %w", err)
	}

	row, err := s.queries.CreateReceipt(ctx, sqlc.CreateReceiptParams{
		UserID:       userID,
		ImagePath:    key,
		ImageHash:    imageHash,
		ImageTakenAt: imageTakenAt,
		Status:       int16(pb.ReceiptStatus_RECEIPT_STATUS_PENDING),
	})
	if err != nil {
		if delErr := s.store.Delete(ctx, key); delErr != nil {
			s.log.Warn("failed to clean up orphaned receipt blob", "key", key, "error", delErr)
		}
		return nil, wrapErr("ReceiptService.Upload", err)
	}

	return receiptToPb(&row, nil), nil
}

func (s *rcptSvc) Create(ctx context.Context, userID uuid.UUID, req *pb.CreateReceiptRequest) (*pb.Receipt, error) {
	status := pb.ReceiptStatus_RECEIPT_STATUS_PARSED
	if req.TransactionId != nil {
		status = pb.ReceiptStatus_RECEIPT_STATUS_LINKED
	}

	var receiptDate *time.Time
	if d := req.GetReceiptDate(); d != nil {
		t := time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.UTC)
		receiptDate = &t
	}

	row, err := s.queries.CreateReceiptRecord(ctx, sqlc.CreateReceiptRecordParams{
		UserID:        userID,
		TransactionID: req.TransactionId,
		Merchant:      req.Merchant,
		ReceiptDate:   receiptDate,
		Currency:      req.Currency,
		SubtotalCents: req.SubtotalCents,
		TaxCents:      req.TaxCents,
		TotalCents:    req.TotalCents,
		Status:        int16(status),
	})
	if err != nil {
		return nil, wrapErr("ReceiptService.Create", err)
	}

	currency := receiptCurrencyOrDefault(row.Currency)
	for i, item := range req.Items {
		_, err := s.queries.CreateReceiptItem(ctx, sqlc.CreateReceiptItemParams{
			ReceiptID:      row.ID,
			RawName:        item.RawName,
			Name:           item.Name,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
			UnitCurrency:   currency,
			SortOrder:      int32(i),
		})
		if err != nil {
			return nil, wrapErr("ReceiptService.Create.CreateItem", err)
		}
	}

	items, err := s.queries.ListReceiptItems(ctx, row.ID)
	if err != nil {
		return nil, wrapErr("ReceiptService.Create.ListItems", err)
	}
	return receiptToPb(&row, items), nil
}

func (s *rcptSvc) Get(ctx context.Context, userID uuid.UUID, id int64) (*pb.Receipt, []*pb.ReceiptLinkCandidate, []byte, error) {
	row, err := s.queries.GetReceipt(ctx, sqlc.GetReceiptParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return nil, nil, nil, wrapErr("ReceiptService.Get", err)
	}

	items, err := s.queries.ListReceiptItems(ctx, row.ID)
	if err != nil {
		return nil, nil, nil, wrapErr("ReceiptService.Get.Items", err)
	}

	receipt := receiptToPb(&row, items)

	var imageData []byte
	if row.ImagePath != nil {
		imageData, err = s.store.Get(ctx, *row.ImagePath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("ReceiptService.Get: %w", err)
		}
	}

	var candidates []*pb.ReceiptLinkCandidate
	if row.Status == pb.ReceiptStatus_RECEIPT_STATUS_PARSED && row.TransactionID == nil && row.TotalCents != nil && row.Currency != nil {
		rows, err := s.queries.FindReceiptLinkCandidates(ctx, sqlc.FindReceiptLinkCandidatesParams{
			UserID:      userID,
			AmountCents: *row.TotalCents,
			Currency:    *row.Currency,
			BestDate:    receiptBestDate(row.ImageTakenAt, row.ReceiptDate, &row.CreatedAt),
		})
		if err != nil {
			s.log.Warn("failed to find link candidates", "receipt_id", id, "error", err)
		} else {
			candidates = make([]*pb.ReceiptLinkCandidate, len(rows))
			for i, r := range rows {
				candidates[i] = linkCandidateToPb(r, *row.TotalCents)
			}
		}
	}

	return receipt, candidates, imageData, nil
}

func (s *rcptSvc) List(ctx context.Context, userID uuid.UUID, req *pb.ListReceiptsRequest) ([]*pb.Receipt, int64, error) {
	params := buildListReceiptsParams(userID, req)

	rows, err := s.queries.ListReceipts(ctx, params)
	if err != nil {
		return nil, 0, wrapErr("ReceiptService.List", err)
	}

	var totalCount int64
	receipts := make([]*pb.Receipt, len(rows))
	for i := range rows {
		if i == 0 {
			totalCount = rows[i].TotalCount
		}

		items, err := s.queries.ListReceiptItems(ctx, rows[i].ID)
		if err != nil {
			return nil, 0, wrapErr("ReceiptService.List.Items", err)
		}

		receipts[i] = receiptListRowToPb(&rows[i], items)
	}

	return receipts, totalCount, nil
}

func (s *rcptSvc) Update(ctx context.Context, userID uuid.UUID, id int64, req *pb.UpdateReceiptRequest) (*pb.Receipt, error) {
	params := buildUpdateReceiptParams(userID, id, req)

	row, err := s.queries.UpdateReceipt(ctx, params)
	if err != nil {
		return nil, wrapErr("ReceiptService.Update", err)
	}

	// if items provided, replace all items
	if len(req.Items) > 0 {
		if err := s.queries.DeleteReceiptItemsByReceipt(ctx, row.ID); err != nil {
			return nil, wrapErr("ReceiptService.Update.DeleteItems", err)
		}
		for i, item := range req.Items {
			currency := receiptCurrencyOrDefault(row.Currency)
			_, err := s.queries.CreateReceiptItem(ctx, sqlc.CreateReceiptItemParams{
				ReceiptID:      row.ID,
				RawName:        item.RawName,
				Name:           item.Name,
				Quantity:       item.Quantity,
				UnitPriceCents: item.UnitPriceCents,
				UnitCurrency:   currency,
				SortOrder:      int32(i),
			})
			if err != nil {
				return nil, wrapErr("ReceiptService.Update.CreateItem", err)
			}
		}
	}

	items, err := s.queries.ListReceiptItems(ctx, row.ID)
	if err != nil {
		return nil, wrapErr("ReceiptService.Update.ListItems", err)
	}

	return receiptToPb(&row, items), nil
}

func (s *rcptSvc) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	row, err := s.queries.GetReceipt(ctx, sqlc.GetReceiptParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return wrapErr("ReceiptService.Delete", err)
	}

	if err := s.queries.DeleteReceipt(ctx, sqlc.DeleteReceiptParams{
		ID:     id,
		UserID: userID,
	}); err != nil {
		return wrapErr("ReceiptService.Delete", err)
	}

	if row.ImagePath != nil {
		if err := s.store.Delete(ctx, *row.ImagePath); err != nil {
			s.log.Warn("failed to remove receipt image", "key", *row.ImagePath, "error", err)
		}
	}

	return nil
}

func (s *rcptSvc) RetryParsing(ctx context.Context, userID uuid.UUID, id int64) (*pb.Receipt, error) {
	if s.ocrClient == nil {
		return nil, fmt.Errorf("ReceiptService.RetryParsing: OCR not configured: %w", ErrValidation)
	}

	row, err := s.queries.GetReceipt(ctx, sqlc.GetReceiptParams{ID: id, UserID: userID})
	if err != nil {
		return nil, wrapErr("ReceiptService.RetryParsing", err)
	}

	status := pb.ReceiptStatus(row.Status)
	if status == pb.ReceiptStatus_RECEIPT_STATUS_PENDING {
		return nil, fmt.Errorf("ReceiptService.RetryParsing: receipt is already queued for parsing: %w", ErrValidation)
	}

	if err := s.queries.DeleteReceiptItemsByReceipt(ctx, row.ID); err != nil {
		return nil, wrapErr("ReceiptService.RetryParsing.DeleteItems", err)
	}

	updated, err := s.queries.ResetReceiptForRetry(ctx, sqlc.ResetReceiptForRetryParams{ID: id, UserID: userID})
	if err != nil {
		return nil, wrapErr("ReceiptService.RetryParsing", err)
	}

	return receiptToPb(&updated, nil), nil
}

// ----- background worker -------------------------------------------------------------------

func (s *rcptSvc) StartWorker(ctx context.Context) {
	if s.ocrClient == nil {
		s.log.Warn("OCR client not configured, receipt worker disabled")
		return
	}

	s.log.Info("receipt OCR worker started")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("receipt OCR worker stopped")
			return
		case <-ticker.C:
			s.processPendingReceipts(ctx)
			s.retryUnlinkedReceipts(ctx)
		}
	}
}

func (s *rcptSvc) processPendingReceipts(ctx context.Context) {
	pending, err := s.queries.GetPendingReceipts(ctx)
	if err != nil {
		s.log.Error("failed to query pending receipts", "error", err)
		return
	}

	for _, receipt := range pending {
		s.processOneReceipt(ctx, receipt)
	}
}

func (s *rcptSvc) retryUnlinkedReceipts(ctx context.Context) {
	receipts, err := s.queries.GetParsedUnlinkedReceipts(ctx)
	if err != nil {
		s.log.Error("failed to query unlinked receipts", "error", err)
		return
	}

	for _, receipt := range receipts {
		bestDate := receiptBestDate(receipt.ImageTakenAt, receipt.ReceiptDate, &receipt.CreatedAt)
		candidates, err := s.queries.FindReceiptLinkCandidates(ctx, sqlc.FindReceiptLinkCandidatesParams{
			UserID:      receipt.UserID,
			AmountCents: *receipt.TotalCents,
			Currency:    *receipt.Currency,
			BestDate:    bestDate,
		})
		if err != nil {
			s.log.Warn("failed to find link candidates for unlinked receipt", "receipt_id", receipt.ID, "error", err)
			continue
		}

		if match, ok := selectAutoLinkMatch(candidates, *receipt.TotalCents, bestDate); ok {
			linkedStatus := int16(pb.ReceiptStatus_RECEIPT_STATUS_LINKED)
			_, err := s.queries.UpdateReceipt(ctx, sqlc.UpdateReceiptParams{
				ID:            receipt.ID,
				UserID:        receipt.UserID,
				TransactionID: &match.ID,
				Status:        &linkedStatus,
			})
			if err != nil {
				s.log.Warn("auto-link retry failed", "receipt_id", receipt.ID, "transaction_id", match.ID, "error", err)
			} else {
				s.log.Info("receipt auto-linked on retry", "receipt_id", receipt.ID, "transaction_id", match.ID)
			}
		}
	}
}

func (s *rcptSvc) processOneReceipt(ctx context.Context, receipt sqlc.Receipt) {
	if receipt.ImagePath == nil {
		s.log.Error("pending receipt has no image_path", "id", receipt.ID)
		s.setReceiptFailed(ctx, receipt)
		return
	}
	imageData, err := s.store.Get(ctx, *receipt.ImagePath)
	if err != nil {
		s.log.Error("failed to read receipt image", "id", receipt.ID, "key", *receipt.ImagePath, "error", err)
		s.setReceiptFailed(ctx, receipt)
		return
	}

	contentType := extToContentType(path.Ext(*receipt.ImagePath))

	ocrCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	resp, err := s.ocrClient.ParseReceipt(ocrCtx, connect.NewRequest(&pb.ParseReceiptRequest{
		ImageData:   imageData,
		ContentType: contentType,
	}))
	if err != nil {
		s.log.Error("OCR request failed", "id", receipt.ID, "error", err)
		s.setReceiptFailed(ctx, receipt)
		return
	}

	if !resp.Msg.Success || resp.Msg.Data == nil {
		errMsg := "unknown"
		if resp.Msg.Error != nil {
			errMsg = resp.Msg.Error.Message
		}
		s.log.Error("OCR parsing failed", "id", receipt.ID, "error", errMsg)
		s.setReceiptFailed(ctx, receipt)
		return
	}

	parsed := resp.Msg.Data

	updateParams := sqlc.UpdateReceiptParams{
		ID:     receipt.ID,
		UserID: receipt.UserID,
	}

	parsedStatus := int16(pb.ReceiptStatus_RECEIPT_STATUS_PARSED)
	updateParams.Status = &parsedStatus

	if parsed.Merchant != nil {
		updateParams.Merchant = parsed.Merchant
	}
	if parsed.Currency != nil {
		updateParams.Currency = parsed.Currency
	}
	if parsed.Confidence > 0 {
		updateParams.Confidence = &parsed.Confidence
	}

	if parsed.Date != nil {
		t, parseErr := time.Parse("2006-01-02", *parsed.Date)
		if parseErr == nil {
			updateParams.ReceiptDate = &t
		}
	}

	currency := receiptCurrencyOrDefault(parsed.Currency)

	if parsed.Subtotal != nil {
		cents := dollarsToCents(*parsed.Subtotal)
		updateParams.SubtotalCents = &cents
	}
	if parsed.Tax != nil {
		cents := dollarsToCents(*parsed.Tax)
		updateParams.TaxCents = &cents
	}
	if parsed.Total != nil {
		cents := dollarsToCents(*parsed.Total)
		updateParams.TotalCents = &cents
	}

	if _, err := s.queries.UpdateReceipt(ctx, updateParams); err != nil {
		s.log.Error("failed to update receipt from OCR", "id", receipt.ID, "error", err)
		return
	}

	// insert parsed items
	for i, item := range parsed.Items {
		_, err := s.queries.CreateReceiptItem(ctx, sqlc.CreateReceiptItemParams{
			ReceiptID:      receipt.ID,
			RawName:        item.Raw,
			Name:           item.Name,
			Quantity:       item.Qty,
			UnitPriceCents: dollarsToCents(item.UnitPrice),
			UnitCurrency:   currency,
			SortOrder:      int32(i),
		})
		if err != nil {
			s.log.Error("failed to create receipt item from OCR", "receipt_id", receipt.ID, "error", err)
		}
	}

	s.log.Info("receipt parsed successfully", "id", receipt.ID, "merchant", parsed.GetMerchant(), "items", len(parsed.Items))

	// attempt auto-link: exactly one exact-amount match within 7 days of best date
	if parsed.Total != nil && parsed.Currency != nil {
		totalCents := dollarsToCents(*parsed.Total)
		bestDate := receiptBestDate(receipt.ImageTakenAt, updateParams.ReceiptDate, &receipt.CreatedAt)
		candidates, err := s.queries.FindReceiptLinkCandidates(ctx, sqlc.FindReceiptLinkCandidatesParams{
			UserID:      receipt.UserID,
			AmountCents: totalCents,
			Currency:    *parsed.Currency,
			BestDate:    bestDate,
		})
		if err != nil {
			s.log.Warn("auto-link candidate query failed", "receipt_id", receipt.ID, "error", err)
			return
		}

		if match, ok := selectAutoLinkMatch(candidates, totalCents, bestDate); ok {
			linkedStatus := int16(pb.ReceiptStatus_RECEIPT_STATUS_LINKED)
			_, err := s.queries.UpdateReceipt(ctx, sqlc.UpdateReceiptParams{
				ID:            receipt.ID,
				UserID:        receipt.UserID,
				TransactionID: &match.ID,
				Status:        &linkedStatus,
			})
			if err != nil {
				s.log.Warn("auto-link failed", "receipt_id", receipt.ID, "transaction_id", match.ID, "error", err)
			} else {
				s.log.Info("receipt auto-linked", "receipt_id", receipt.ID, "transaction_id", match.ID)
			}
		}
	}
}

func (s *rcptSvc) setReceiptFailed(ctx context.Context, receipt sqlc.Receipt) {
	failedStatus := int16(pb.ReceiptStatus_RECEIPT_STATUS_FAILED)
	_, err := s.queries.UpdateReceipt(ctx, sqlc.UpdateReceiptParams{
		ID:     receipt.ID,
		UserID: receipt.UserID,
		Status: &failedStatus,
	})
	if err != nil {
		s.log.Error("failed to set receipt status to FAILED", "id", receipt.ID, "error", err)
	}
}
