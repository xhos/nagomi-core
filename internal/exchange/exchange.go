package exchange

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateEntry struct {
	rate      float64
	fetchedAt time.Time
}

type Client struct {
	baseURL        string
	httpClient     *http.Client
	supportedCodes map[string]bool
	currencyNames  map[string]string
	codesLoaded    bool

	rateCacheMu  sync.RWMutex
	rateCache    map[string]rateEntry
	rateCacheTTL time.Duration
}

type RatesResponse struct {
	Base  string             `json:"base"`
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"`
}

type CurrenciesResponse map[string]string

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		supportedCodes: make(map[string]bool),
		currencyNames:  make(map[string]string),
		codesLoaded:    false,
		rateCache:      make(map[string]rateEntry),
		rateCacheTTL:   1 * time.Hour,
	}
}

// loadSupportedCurrencies fetches and caches supported currency codes
func (c *Client) loadSupportedCurrencies() error {
	if c.codesLoaded {
		return nil
	}

	url := fmt.Sprintf("%s/currencies", c.baseURL)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch supported currencies: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d when fetching currencies", resp.StatusCode)
	}

	var currencies CurrenciesResponse
	err = json.NewDecoder(resp.Body).Decode(&currencies)
	if err != nil {
		return fmt.Errorf("failed to parse currencies response: %w", err)
	}

	for code, name := range currencies {
		c.supportedCodes[code] = true
		c.currencyNames[code] = name
	}
	c.codesLoaded = true

	return nil
}

// validateCurrency checks if a currency code is valid and supported
func (c *Client) validateCurrency(currencyCode string) error {
	if currencyCode == "" {
		return fmt.Errorf("currency code cannot be empty")
	}

	err := c.loadSupportedCurrencies()
	if err != nil {
		return fmt.Errorf("failed to load supported currencies: %w", err)
	}

	isSupported := c.supportedCodes[currencyCode]
	if !isSupported {
		return fmt.Errorf("currency code '%s' is not supported", currencyCode)
	}

	return nil
}

// IsValidCurrency checks if a currency code is supported by the exchange API
func (c *Client) IsValidCurrency(currencyCode string) (bool, error) {
	err := c.validateCurrency(currencyCode)
	if err != nil {
		if strings.Contains(err.Error(), "is not supported") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetCurrencies returns all supported currency codes and their names.
func (c *Client) GetCurrencies() (map[string]string, error) {
	if err := c.loadSupportedCurrencies(); err != nil {
		return nil, err
	}
	return c.currencyNames, nil
}

// GetExchangeRate fetches exchange rate from one currency to another
// If date is nil, gets latest rate. Otherwise gets historical rate for the specified date.
func (c *Client) GetExchangeRate(fromCurrency, toCurrency string, date *time.Time) (float64, error) {
	err := c.validateCurrency(fromCurrency)
	if err != nil {
		return 0, fmt.Errorf("invalid from currency: %w", err)
	}

	err = c.validateCurrency(toCurrency)
	if err != nil {
		return 0, fmt.Errorf("invalid to currency: %w", err)
	}

	if fromCurrency == toCurrency {
		return 1.0, nil
	}

	isHistorical := date != nil

	if isHistorical {
		tomorrow := time.Now().AddDate(0, 0, 1)
		if date.After(tomorrow) {
			return 0, fmt.Errorf("cannot get exchange rates for future dates")
		}

		earliestDate := time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC)
		if date.Before(earliestDate) {
			return 0, fmt.Errorf("exchange rates not available before 1999-01-04")
		}
	}

	// Build cache key: "USD:EUR:2024-01-15" or "USD:EUR:latest"
	dateKey := "latest"
	if isHistorical {
		dateKey = date.Format("2006-01-02")
	}
	cacheKey := fromCurrency + ":" + toCurrency + ":" + dateKey

	// Check cache
	c.rateCacheMu.RLock()
	if entry, ok := c.rateCache[cacheKey]; ok && time.Since(entry.fetchedAt) < c.rateCacheTTL {
		c.rateCacheMu.RUnlock()
		return entry.rate, nil
	}
	c.rateCacheMu.RUnlock()

	var url string
	if isHistorical {
		url = fmt.Sprintf("%s/%s?base=%s&symbols=%s", c.baseURL, dateKey, fromCurrency, toCurrency)
	} else {
		url = fmt.Sprintf("%s/latest?base=%s&symbols=%s", c.baseURL, fromCurrency, toCurrency)
	}

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch exchange rate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d when fetching exchange rate", resp.StatusCode)
	}

	var ratesResp RatesResponse
	err = json.NewDecoder(resp.Body).Decode(&ratesResp)
	if err != nil {
		return 0, fmt.Errorf("failed to parse rates response: %w", err)
	}

	rate, exists := ratesResp.Rates[toCurrency]
	if !exists {
		return 0, fmt.Errorf("exchange rate not found for %s to %s", fromCurrency, toCurrency)
	}

	// Store in cache
	c.rateCacheMu.Lock()
	c.rateCache[cacheKey] = rateEntry{rate: rate, fetchedAt: time.Now()}
	c.rateCacheMu.Unlock()

	return rate, nil
}
