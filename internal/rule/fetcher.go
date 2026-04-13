package rule

import (
	"github.com/teacat99/SubForge/internal/model"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type rulePayload struct {
	Payload []string `yaml:"payload"`
}

func FetchRules(sg *model.ServiceGroup) error {
	if sg.RuleType == "local" {
		var payload rulePayload
		if err := yaml.Unmarshal([]byte(sg.CachedRules), &payload); err == nil {
			sg.RuleCount = len(payload.Payload)
		}
		now := time.Now()
		sg.LastFetched = &now
		return nil
	}

	urls := splitURLs(sg.RuleURL)
	if len(urls) == 0 {
		return fmt.Errorf("no rule URLs configured")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	var merged []string

	for _, u := range urls {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return fmt.Errorf("build request for %s: %w", u, err)
		}
		req.Header.Set("User-Agent", "SubForge/1.0")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", u, err)
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("fetch %s: HTTP %d", u, resp.StatusCode)
		}
		if err != nil {
			return fmt.Errorf("read body from %s: %w", u, err)
		}

		var payload rulePayload
		if err := yaml.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("parse rules YAML from %s: %w", u, err)
		}
		merged = append(merged, payload.Payload...)
	}

	combined := rulePayload{Payload: merged}
	out, err := yaml.Marshal(&combined)
	if err != nil {
		return fmt.Errorf("marshal merged rules: %w", err)
	}

	now := time.Now()
	sg.CachedRules = string(out)
	sg.RuleCount = len(merged)
	sg.LastFetched = &now
	return nil
}

func splitURLs(raw string) []string {
	var urls []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			urls = append(urls, line)
		}
	}
	return urls
}

// ParseCachedRules extracts rules from the cached YAML payload
// and appends the group name to each rule for Clash config generation.
func ParseCachedRules(cached string, groupName string) []string {
	if cached == "" {
		return nil
	}
	var payload rulePayload
	if err := yaml.Unmarshal([]byte(cached), &payload); err != nil {
		return nil
	}

	rules := make([]string, 0, len(payload.Payload))
	for _, r := range payload.Payload {
		r = strings.TrimSpace(r)
		if r == "" || strings.HasPrefix(r, "#") {
			continue
		}
		if strings.HasSuffix(r, ",no-resolve") {
			base := strings.TrimSuffix(r, ",no-resolve")
			rules = append(rules, fmt.Sprintf("%s,%s,no-resolve", base, groupName))
		} else {
			rules = append(rules, fmt.Sprintf("%s,%s", r, groupName))
		}
	}
	return rules
}
