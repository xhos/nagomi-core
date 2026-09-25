package exchange

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrUnsupportedCurrency means the exchange API has no rates for the currency.
var ErrUnsupportedCurrency = errors.New("not supported")

type rateEntry struct {
	rate      float64
	fetchedAt time.Time
}

type Client struct {
	baseURL         string
	httpClient      *http.Client
	supportedCodes  map[string]bool
	currencyNames   map[string]string
	codesLoaded     bool
	currencyCacheMu sync.Mutex

	rateCacheMu  sync.RWMutex
	rateCache    map[string]rateEntry
	rateCacheTTL time.Duration
}

// Rate is one entry of a Frankfurter v2 /rates response.
type Rate struct {
	Date  string  `json:"date"`
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
}

// Currency is one entry of a Frankfurter v2 /currencies response.
type Currency struct {
	ISOCode string `json:"iso_code"`
	Name    string `json:"name"`
}

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
	c.currencyCacheMu.Lock()
	defer c.currencyCacheMu.Unlock()
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

	var currencies []Currency
	err = json.NewDecoder(resp.Body).Decode(&currencies)
	if err != nil {
		return fmt.Errorf("failed to parse currencies response: %w", err)
	}

	for _, cur := range currencies {
		c.supportedCodes[cur.ISOCode] = true
		c.currencyNames[cur.ISOCode] = cur.Name
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
		return fmt.Errorf("currency code '%s' is %w", currencyCode, ErrUnsupportedCurrency)
	}

	return nil
}

// IsValidCurrency checks if a currency code is supported by the exchange API
func (c *Client) IsValidCurrency(currencyCode string) (bool, error) {
	err := c.validateCurrency(currencyCode)
	if err != nil {
		if errors.Is(err, ErrUnsupportedCurrency) {
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

	// A historical date resolves to the latest published rate on or before it.
	url := fmt.Sprintf("%s/rates?base=%s&quotes=%s", c.baseURL, fromCurrency, toCurrency)
	if isHistorical {
		url += "&date=" + dateKey
	}

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch exchange rate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d when fetching exchange rate", resp.StatusCode)
	}

	var rates []Rate
	err = json.NewDecoder(resp.Body).Decode(&rates)
	if err != nil {
		return 0, fmt.Errorf("failed to parse rates response: %w", err)
	}

	if len(rates) == 0 || rates[0].Quote != toCurrency {
		return 0, fmt.Errorf("exchange rate not found for %s to %s", fromCurrency, toCurrency)
	}
	rate := rates[0].Rate

	// Store in cache
	c.rateCacheMu.Lock()
	c.rateCache[cacheKey] = rateEntry{rate: rate, fetchedAt: time.Now()}
	c.rateCacheMu.Unlock()

	return rate, nil
}
