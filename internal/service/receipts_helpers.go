package service

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"time"

	"nagomi-core/internal/db/sqlc"
	pb "nagomi-core/internal/gen/nagomi/v1"

	"github.com/google/uuid"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"google.golang.org/genproto/googleapis/type/money"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var contentTypeToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/heic": "heic",
}

func extToContentType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".heic":
		return "image/heic"
	default:
		return "application/octet-stream"
	}
}

const maxImageDim = 1920

const autoLinkAmountTolerance = 0

const autoLinkDateWindow = 7 * 24 * time.Hour

func resizeImage(data []byte, contentType string) ([]byte, string) {
	if contentType == "image/heic" {
		return data, contentType
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, contentType
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	longest := width
	if height > longest {
		longest = height
	}
	if longest <= maxImageDim {
		return data, contentType
	}

	scale := float64(maxImageDim) / float64(longest)
	newWidth := int(math.Round(float64(width) * scale))
	newHeight := int(math.Round(float64(height) * scale))

	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if contentType == "image/png" {
		if err := png.Encode(&buf, dst); err != nil {
			return data, contentType
		}
		return buf.Bytes(), "image/png"
	}

	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return data, contentType
	}
	return buf.Bytes(), "image/jpeg"
}

func buildListReceiptsParams(userID uuid.UUID, req *pb.ListReceiptsRequest) sqlc.ListReceiptsParams {
	params := sqlc.ListReceiptsParams{UserID: userID}
	if req.Limit != nil {
		params.Lim = req.Limit
	}
	if req.Offset != nil {
		params.Off = req.Offset
	}
	if req.Status != nil {
		status := int16(*req.Status)
		params.Status = &status
	}
	if req.UnlinkedOnly != nil {
		params.UnlinkedOnly = req.UnlinkedOnly
	}
	if req.StartDate != nil {
		start := dateToTime(req.StartDate)
		params.StartDate = start
	}
	if req.EndDate != nil {
		end := dateToTime(req.EndDate)
		params.EndDate = end
	}
	if req.Query != nil {
		params.Query = req.Query
	}
	if req.MinTotalCents != nil {
		params.MinTotalCents = req.MinTotalCents
	}
	if req.MaxTotalCents != nil {
		params.MaxTotalCents = req.MaxTotalCents
	}
	if req.Currency != nil {
		params.Currency = req.Currency
	}
	return params
}

func buildUpdateReceiptParams(userID uuid.UUID, id int64, req *pb.UpdateReceiptRequest) sqlc.UpdateReceiptParams {
	params := sqlc.UpdateReceiptParams{ID: id, UserID: userID}
	if req.TransactionId != nil {
		params.TransactionID = req.TransactionId
		linkedStatus := int16(pb.ReceiptStatus_RECEIPT_STATUS_LINKED)
		params.Status = &linkedStatus
	}
	return params
}

func receiptCurrencyOrDefault(currency *string) string {
	if currency != nil {
		return *currency
	}
	return "CAD"
}

func selectAutoLinkMatch(candidates []sqlc.FindReceiptLinkCandidatesRow, totalCents int64, bestDate *time.Time) (sqlc.FindReceiptLinkCandidatesRow, bool) {
	exactMatches := 0
	var match sqlc.FindReceiptLinkCandidatesRow
	for _, candidate := range candidates {
		if absDiff(candidate.TxAmountCents, totalCents) > autoLinkAmountTolerance {
			continue
		}
		if bestDate != nil && absDuration(candidate.TxDate.Sub(*bestDate)) > autoLinkDateWindow {
			continue
		}
		exactMatches++
		match = candidate
	}
	if exactMatches != 1 {
		return sqlc.FindReceiptLinkCandidatesRow{}, false
	}
	return match, true
}

func extractEXIFDate(imageData []byte) *time.Time {
	x, err := exif.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil
	}
	t, err := x.DateTime()
	if err != nil {
		return nil
	}
	return &t
}

func absDiff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func receiptBestDate(imageTakenAt *time.Time, receiptDate *time.Time, createdAt *time.Time) *time.Time {
	if imageTakenAt != nil {
		return imageTakenAt
	}
	if receiptDate != nil {
		return receiptDate
	}
	return createdAt
}

func dollarsToCents(dollars float64) int64 {
	return int64(math.Round(dollars * 100))
}

func receiptToPb(r *sqlc.Receipt, items []sqlc.ReceiptItem) *pb.Receipt {
	proto := &pb.Receipt{
		Id:            r.ID,
		UserId:        r.UserID.String(),
		TransactionId: r.TransactionID,
		ImagePath:     r.ImagePath,
		Merchant:      r.Merchant,
		Currency:      r.Currency,
		Confidence:    r.Confidence,
		Status:        pb.ReceiptStatus(r.Status),
		CreatedAt:     timestamppb.New(r.CreatedAt),
		UpdatedAt:     timestamppb.New(r.UpdatedAt),
		Source:        r.Source,
	}

	if r.ReceiptDate != nil {
		proto.ReceiptDate = timeToDate(*r.ReceiptDate)
	}
	if r.ImageTakenAt != nil {
		proto.ImageTakenAt = timestamppb.New(*r.ImageTakenAt)
	}
	bestDate := receiptBestDate(r.ImageTakenAt, r.ReceiptDate, &r.CreatedAt)
	if bestDate != nil {
		proto.BestDate = timeToDate(*bestDate)
	}

	currency := receiptCurrencyOrDefault(r.Currency)
	if r.SubtotalCents != nil {
		proto.Subtotal = centsToMoney(*r.SubtotalCents, currency)
	}
	if r.TaxCents != nil {
		proto.Tax = centsToMoney(*r.TaxCents, currency)
	}
	if r.TotalCents != nil {
		proto.Total = centsToMoney(*r.TotalCents, currency)
	}

	proto.Items = make([]*pb.ReceiptItem, len(items))
	for i := range items {
		proto.Items[i] = receiptItemToPb(&items[i])
	}

	return proto
}

func receiptListRowToPb(r *sqlc.ListReceiptsRow, items []sqlc.ReceiptItem) *pb.Receipt {
	proto := &pb.Receipt{
		Id:            r.ID,
		UserId:        r.UserID.String(),
		TransactionId: r.TransactionID,
		ImagePath:     r.ImagePath,
		Merchant:      r.Merchant,
		Currency:      r.Currency,
		Confidence:    r.Confidence,
		Status:        pb.ReceiptStatus(r.Status),
		CreatedAt:     timestamppb.New(r.CreatedAt),
		UpdatedAt:     timestamppb.New(r.UpdatedAt),
		Source:        r.Source,
	}

	if r.ReceiptDate != nil {
		proto.ReceiptDate = timeToDate(*r.ReceiptDate)
	}
	if r.ImageTakenAt != nil {
		proto.ImageTakenAt = timestamppb.New(*r.ImageTakenAt)
	}
	bestDate := receiptBestDate(r.ImageTakenAt, r.ReceiptDate, &r.CreatedAt)
	if bestDate != nil {
		proto.BestDate = timeToDate(*bestDate)
	}

	currency := receiptCurrencyOrDefault(r.Currency)
	if r.SubtotalCents != nil {
		proto.Subtotal = centsToMoney(*r.SubtotalCents, currency)
	}
	if r.TaxCents != nil {
		proto.Tax = centsToMoney(*r.TaxCents, currency)
	}
	if r.TotalCents != nil {
		proto.Total = centsToMoney(*r.TotalCents, currency)
	}

	proto.Items = make([]*pb.ReceiptItem, len(items))
	for i := range items {
		proto.Items[i] = receiptItemToPb(&items[i])
	}

	return proto
}

func linkCandidateToPb(r sqlc.FindReceiptLinkCandidatesRow, receiptTotalCents int64) *pb.ReceiptLinkCandidate {
	candidate := &pb.ReceiptLinkCandidate{
		TransactionId:   r.ID,
		AccountId:       r.AccountID,
		AccountName:     r.AccountDisplayName,
		Amount:          centsToMoney(r.TxAmountCents, r.TxCurrency),
		TxDate:          timestamppb.New(r.TxDate),
		AmountDiffCents: r.TxAmountCents - receiptTotalCents,
	}
	if r.Merchant != nil {
		candidate.Merchant = *r.Merchant
	}
	if days, ok := r.DateDiffDays.(int32); ok {
		candidate.DateDiffDays = days
	} else if days, ok := r.DateDiffDays.(int64); ok {
		candidate.DateDiffDays = int32(days)
	}
	return candidate
}

func receiptItemToPb(item *sqlc.ReceiptItem) *pb.ReceiptItem {
	return &pb.ReceiptItem{
		Id:        item.ID,
		ReceiptId: item.ReceiptID,
		RawName:   item.RawName,
		Name:      item.Name,
		Quantity:  item.Quantity,
		UnitPrice: &money.Money{
			CurrencyCode: item.UnitCurrency,
			Units:        item.UnitPriceCents / 100,
			Nanos:        int32((item.UnitPriceCents % 100) * 10_000_000),
		},
		SortOrder: item.SortOrder,
	}
}
