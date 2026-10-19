package sec

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const secBase = "https://data.sec.gov"
const tickerURL = "https://www.sec.gov/files/company_tickers.json"

type Client struct {
	http      *http.Client
	userAgent string
}

func NewClient(userAgent string) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
		userAgent: userAgent,
	}
}

func (c *Client) Get(
	ctx context.Context,
	url string,
	dst any,
) error {
	var lastErr error

	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) *
				500 * time.Millisecond

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, url, nil,
		)
		if err != nil {
			return err
		}

		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(
			io.LimitReader(resp.Body, 64<<20),
		)
		resp.Body.Close()

		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode >= 500 {
			lastErr = fmt.Errorf(
				"SEC returned HTTP %d", resp.StatusCode,
			)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf(
				"SEC returned HTTP %d: %s",
				resp.StatusCode, strings.TrimSpace(string(body)),
			)
		}

		if err := json.Unmarshal(body, dst); err != nil {
			return fmt.Errorf("decode SEC response: %w", err)
		}

		return nil
	}

	return fmt.Errorf("request failed after retries: %w", lastErr)
}

// ResolveTicker looks up a ticker in the SEC's company ticker mapping and
// returns its zero-padded CIK.
func (c *Client) ResolveTicker(ctx context.Context, ticker string) (string, error) {
	ticker = strings.ToUpper(strings.TrimSpace(ticker))
	if ticker == "" {
		return "", fmt.Errorf("ticker cannot be empty")
	}
	var entries map[string]struct {
		CIK    int    `json:"cik_str"`
		Ticker string `json:"ticker"`
	}
	if err := c.Get(ctx, tickerURL, &entries); err != nil {
		return "", fmt.Errorf("look up ticker %q: %w", ticker, err)
	}
	var match string
	for _, entry := range entries {
		if strings.EqualFold(entry.Ticker, ticker) {
			if match != "" && match != strconv.Itoa(entry.CIK) {
				return "", fmt.Errorf("ticker %q matches multiple companies", ticker)
			}
			match = strconv.Itoa(entry.CIK)
		}
	}
	if match == "" {
		return "", fmt.Errorf("ticker %q not found in SEC company list", ticker)
	}
	return NormaliseCIK(match)
}

func (c *Client) Submissions(
	ctx context.Context, cik string,
) (*Submissions, error) {
	var result Submissions
	url := secBase + "/submissions/CIK" + cik + ".json"

	if err := c.Get(ctx, url, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Facts(
	ctx context.Context, cik string,
) (*CompanyFacts, error) {
	var result CompanyFacts
	url := secBase + "/api/xbrl/companyfacts/CIK" +
		cik + ".json"

	if err := c.Get(ctx, url, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Document(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("SEC returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", err
	}
	return plainText(string(body)), nil
}

var (
	scriptTags = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	styleTags  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	ixHeaders  = regexp.MustCompile(`(?is)<ix:header\b[^>]*>.*?</ix:header\s*>`)
	tags       = regexp.MustCompile(`(?s)<[^>]*>`)
	spaces     = regexp.MustCompile(`[\t\r ]+`)
	blankLines = regexp.MustCompile(`\n{3,}`)
	breaks     = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|tr|h[1-6]|li|section)\s*>`)
)

func plainText(source string) string {
	source = scriptTags.ReplaceAllString(source, " ")
	source = styleTags.ReplaceAllString(source, " ")
	source = ixHeaders.ReplaceAllString(source, " ")
	source = breaks.ReplaceAllString(source, "\n")
	source = tags.ReplaceAllString(source, " ")
	source = html.UnescapeString(source)
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(spaces.ReplaceAllString(line, " "))
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}
