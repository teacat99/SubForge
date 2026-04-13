package generator

import (
	"github.com/teacat99/SubForge/internal/model"
	"github.com/teacat99/SubForge/internal/rule"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Options struct {
	Nodes         []model.Node
	Subscriptions []model.Subscription
	ServiceGroups []model.ServiceGroup
	CatchAll      bool
	GeoipCN       bool
	DnsConfig     string // raw YAML of dns section; empty = use default
}

type clashProxy = map[string]any

func Generate(opts Options) ([]byte, error) {
	subMap := make(map[uint]*model.Subscription)
	for i := range opts.Subscriptions {
		subMap[opts.Subscriptions[i].ID] = &opts.Subscriptions[i]
	}

	proxies, nodeNames := buildProxies(opts.Nodes, subMap)
	if len(proxies) == 0 {
		return nil, fmt.Errorf("no proxies available")
	}
	proxyGroups := buildProxyGroups(opts.ServiceGroups, nodeNames)
	rules := buildRules(opts.ServiceGroups, opts.GeoipCN, opts.CatchAll)

	dnsSection := buildDefaultDNS()
	if opts.DnsConfig != "" {
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(opts.DnsConfig), &parsed); err == nil && len(parsed) > 0 {
			dnsSection = parsed
		}
	}

	config := map[string]any{
		"port":                7890,
		"socks-port":          7891,
		"allow-lan":           true,
		"mode":                "rule",
		"log-level":           "info",
		"external-controller": ":9090",
		"unified-delay":       true,
		"ipv6":                false,
		"proxies":             proxies,
		"proxy-groups":        proxyGroups,
		"rules":               rules,
		"dns":                 dnsSection,
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	return fixYamlUnicode(data), nil
}

func buildProxies(nodes []model.Node, subMap map[uint]*model.Subscription) ([]clashProxy, []string) {
	var proxies []clashProxy
	var names []string
	for _, n := range nodes {
		var p clashProxy
		if err := json.Unmarshal([]byte(n.RawConfig), &p); err != nil {
			continue
		}
		displayName := ComputeDisplayName(n, subMap)
		p["name"] = displayName
		proxies = append(proxies, p)
		names = append(names, displayName)
	}
	return proxies, names
}

func ComputeDisplayName(n model.Node, subMap map[uint]*model.Subscription) string {
	base := n.Name
	if n.Alias != "" {
		base = n.Alias
	}

	sub, ok := subMap[n.SubscriptionID]
	if !ok {
		return base
	}

	suffix := ""
	if sub.AppendSubName && sub.Name != "" {
		suffix += "-" + sub.Name
	}
	if sub.AppendTraffic {
		used := sub.TrafficUsed
		suffix += "-" + formatBytes(used)
	}
	return base + suffix
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	b := float64(bytes)
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0fB", b)
	}
	return fmt.Sprintf("%.1f%s", b, units[i])
}

func buildProxyGroups(groups []model.ServiceGroup, nodeNames []string) []map[string]any {
	selectProxies := append([]string{"节点选择", "DIRECT"}, nodeNames...)

	result := []map[string]any{
		{
			"name":    "节点选择",
			"type":    "select",
			"proxies": append([]string{"自动选择", "DIRECT"}, nodeNames...),
		},
		{
			"name":      "自动选择",
			"type":      "url-test",
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  300,
			"tolerance": 50,
			"proxies":   nodeNames,
		},
	}

	nodeSet := make(map[string]bool, len(nodeNames))
	for _, n := range nodeNames {
		nodeSet[n] = true
	}

	for _, g := range groups {
		var proxies []string
		dp := strings.TrimSpace(g.DefaultProxy)
		switch dp {
		case "DIRECT":
			proxies = append([]string{"DIRECT", "节点选择"}, nodeNames...)
		case "REJECT":
			proxies = append([]string{"REJECT", "DIRECT", "节点选择"}, nodeNames...)
		case "", "自动选择":
			proxies = selectProxies
		default:
			if nodeSet[dp] {
				proxies = append([]string{dp, "节点选择", "DIRECT"}, nodeNames...)
			} else {
				proxies = selectProxies
			}
		}
		result = append(result, map[string]any{
			"name":    g.Name,
			"type":    "select",
			"proxies": proxies,
		})
	}

	result = append(result, map[string]any{
		"name":    "漏网之鱼",
		"type":    "select",
		"proxies": append([]string{"DIRECT", "节点选择"}, nodeNames...),
	})

	return result
}

func buildRules(groups []model.ServiceGroup, geoipCN bool, catchAll bool) []string {
	var rules []string
	for _, g := range groups {
		parsed := rule.ParseCachedRules(g.CachedRules, g.Name)
		rules = append(rules, parsed...)
	}
	if geoipCN {
		rules = append(rules, "GEOIP,CN,DIRECT")
	}
	if catchAll {
		rules = append(rules, "MATCH,漏网之鱼")
	}
	return rules
}

func buildDefaultDNS() map[string]any {
	return map[string]any{
		"enable":        true,
		"ipv6":          false,
		"listen":        ":53",
		"enhanced-mode": "fake-ip",
		"fake-ip-range": "28.0.0.1/8",
		"fake-ip-filter": []string{
			"*.lan", "*.localdomain", "*.localhost", "*.local",
			"+.msftconnecttest.com", "+.msftncsi.com",
			"time.*.com", "ntp.*.com",
			"+.pool.ntp.org",
		},
		"nameserver": []string{
			"https://1.1.1.1/dns-query",
			"https://dns.google/dns-query",
		},
		"default-nameserver": []string{
			"tls://223.5.5.5",
			"tls://119.29.29.29",
		},
		"proxy-server-nameserver": []string{
			"https://doh.pub/dns-query",
		},
		"direct-nameserver": []string{
			"https://doh.pub/dns-query",
			"https://dns.alidns.com/dns-query",
		},
	}
}

var unicodeRe = regexp.MustCompile(`\\U([0-9A-Fa-f]{8})`)

func fixYamlUnicode(data []byte) []byte {
	return unicodeRe.ReplaceAllFunc(data, func(m []byte) []byte {
		cp, err := strconv.ParseInt(string(m[2:]), 16, 32)
		if err != nil {
			return m
		}
		return []byte(string(rune(cp)))
	})
}
