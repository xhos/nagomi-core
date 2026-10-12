package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nagomi-core/internal/exchange"

	"github.com/charmbracelet/log"
)

func TestCheckCurrencies(t *testing.T) {
	up := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`[{"iso_code":"CAD","name":"Canadian Dollar"},{"iso_code":"USD","name":"US Dollar"}]`))
	}))
	defer srv.Close()

	cad, xyz := "CAD", "XYZ"
	svc := &acctSvc{log: log.New(nil), exchange: exchange.NewClient(srv.URL)}
	if err := svc.checkCurrencies(&cad, nil); err != nil {
		t.Errorf("CAD rejected: %v", err)
	}
	if err := svc.checkCurrencies(&cad, &xyz); !errors.Is(err, ErrValidation) {
		t.Errorf("XYZ: err = %v, want validation error", err)
	}

	up = false
	down := &acctSvc{log: log.New(nil), exchange: exchange.NewClient(srv.URL)}
	if err := down.checkCurrencies(&xyz); err != nil {
		t.Errorf("unreachable exchange api blocked the edit: %v", err)
	}
}
