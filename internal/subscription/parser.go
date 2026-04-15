package subscription

import (
	"github.com/teacat99/SubForge/internal/model"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type TrafficInfo struct {
	Upload   int64
	Download int64
	Total    int64
	Expire   int64
}

type clashConfig struct {
	Proxies []map[string]any `yaml:"proxies"`
}

func Fetch(sub *model.Subscription) ([]model.Node, *TrafficInfo, error) {
	result, err := fetchContent(sub.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch: %w", err)
	}

	var info *TrafficInfo
	if h := result.Headers.Get("subscription-userinfo"); h != "" {
		info = parseTrafficHeader(h)
	}

	nodes, err := parseYAML(sub.ID, result.Data)
	if err == nil && len(nodes) > 0 {
		return nodes, info, nil
	}

	nodes, err2 := parseBase64(sub.ID, result.Data)
	if err2 == nil && len(nodes) > 0 {
		return nodes, info, nil
	}

	if err != nil {
		return nil, info, fmt.Errorf("yaml: %w; base64: %w", err, err2)
	}
	return nil, info, fmt.Errorf("no proxies found in any format")
}

type fetchResult struct {
	Data    []byte
	Headers http.Header
}

func fetchContent(rawURL string) (*fetchResult, error) {
	if strings.HasPrefix(rawURL, "file://") {
		data, err := os.ReadFile(strings.TrimPrefix(rawURL, "file://"))
		if err != nil {
			return nil, err
		}
		return &fetchResult{Data: data, Headers: http.Header{}}, nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "clash-verge/v2.0.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &fetchResult{Data: data, Headers: resp.Header}, nil
}

func parseTrafficHeader(header string) *TrafficInfo {
	info := &TrafficInfo{}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		val, _ := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		switch strings.TrimSpace(kv[0]) {
		case "upload":
			info.Upload = val
		case "download":
			info.Download = val
		case "total":
			info.Total = val
		case "expire":
			info.Expire = val
		}
	}
	return info
}

func renameNodes(nodes []model.Node, sub *model.Subscription, info *TrafficInfo) {
	if !sub.AppendSubName && !sub.AppendTraffic {
		return
	}
	suffix := ""
	if sub.AppendSubName && sub.Name != "" {
		suffix += "-" + sub.Name
	}
	if sub.AppendTraffic && info != nil {
		used := info.Upload + info.Download
		suffix += "-" + FormatBytes(used)
	}
	if suffix == "" {
		return
	}
	for i := range nodes {
		newName := nodes[i].Name + suffix
		var raw map[string]any
		if err := json.Unmarshal([]byte(nodes[i].RawConfig), &raw); err == nil {
			raw["name"] = newName
			if b, err := json.Marshal(raw); err == nil {
				nodes[i].RawConfig = string(b)
			}
		}
		nodes[i].Name = newName
	}
}

func FormatBytes(bytes int64) string {
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

func ParseContent(subID uint, data []byte) ([]model.Node, error) {
	nodes, err := parseYAML(subID, data)
	if err == nil && len(nodes) > 0 {
		return nodes, nil
	}
	nodes, err2 := parseBase64(subID, data)
	if err2 == nil && len(nodes) > 0 {
		return nodes, nil
	}
	if err != nil {
		return nil, fmt.Errorf("yaml: %w; base64: %w", err, err2)
	}
	return nil, fmt.Errorf("no proxies found in any format")
}

func parseYAML(subID uint, data []byte) ([]model.Node, error) {
	var cfg clashConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	if len(cfg.Proxies) == 0 {
		return nil, fmt.Errorf("no proxies found")
	}
	return proxiesToNodes(subID, cfg.Proxies), nil
}

func parseBase64(subID uint, data []byte) ([]model.Node, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("base64 decode: %w", err)
		}
	}
	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	var nodes []model.Node
	seen := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var proxy map[string]any
		var err error
		switch {
		case strings.HasPrefix(line, "vless://"):
			proxy, err = parseVlessURI(line)
		case strings.HasPrefix(line, "vmess://"):
			proxy, err = parseVmessURI(line)
		case strings.HasPrefix(line, "ss://"):
			proxy, err = parseSsURI(line)
		case strings.HasPrefix(line, "trojan://"):
			proxy, err = parseTrojanURI(line)
		default:
			continue
		}
		if err != nil || proxy == nil {
			continue
		}
		key := fmt.Sprintf("%s:%s:%v", proxy["type"], proxy["server"], proxy["port"])
		if seen[key] {
			continue
		}
		seen[key] = true
		raw, _ := json.Marshal(proxy)
		nodes = append(nodes, model.Node{
			SubscriptionID: subID,
			Name:           fmt.Sprintf("%v", proxy["name"]),
			Type:           fmt.Sprintf("%v", proxy["type"]),
			Server:         fmt.Sprintf("%v", proxy["server"]),
			Port:           toInt(proxy["port"]),
			RawConfig:      string(raw),
			Enabled:        true,
		})
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no valid proxy URIs found")
	}
	return nodes, nil
}

func parseVlessURI(uri string) (map[string]any, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(u.Port())
	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}
	p := map[string]any{
		"name":       name,
		"type":       "vless",
		"server":     u.Hostname(),
		"port":       port,
		"uuid":       u.User.Username(),
		"encryption": "none",
		"udp":        true,
	}
	q := u.Query()
	if q.Get("security") == "tls" || q.Get("security") == "reality" {
		p["tls"] = true
		if sni := q.Get("sni"); sni != "" {
			p["sni"] = sni
			p["servername"] = sni
		}
	}
	if net := q.Get("type"); net != "" {
		p["network"] = net
		switch net {
		case "ws":
			opts := map[string]any{}
			if path := q.Get("path"); path != "" {
				opts["path"] = path
			}
			if host := q.Get("host"); host != "" {
				opts["headers"] = map[string]any{"Host": host}
			}
			p["ws-opts"] = opts
		case "grpc":
			if sn := q.Get("serviceName"); sn != "" {
				p["grpc-opts"] = map[string]any{"grpc-service-name": sn}
			}
		}
	}
	return p, nil
}

func parseVmessURI(uri string) (map[string]any, error) {
	raw := strings.TrimPrefix(uri, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil, err
		}
	}
	var v map[string]any
	if err := json.Unmarshal(decoded, &v); err != nil {
		return nil, err
	}
	port := toInt(v["port"])
	name, _ := v["ps"].(string)
	server, _ := v["add"].(string)
	if name == "" {
		name = server
	}
	p := map[string]any{
		"name":    name,
		"type":    "vmess",
		"server":  server,
		"port":    port,
		"uuid":    v["id"],
		"alterId": toInt(v["aid"]),
		"cipher":  "auto",
		"udp":     true,
	}
	if tls, _ := v["tls"].(string); tls == "tls" {
		p["tls"] = true
		if sni, _ := v["sni"].(string); sni != "" {
			p["servername"] = sni
		}
	}
	if net, _ := v["net"].(string); net != "" {
		p["network"] = net
		switch net {
		case "ws":
			opts := map[string]any{}
			if path, _ := v["path"].(string); path != "" {
				opts["path"] = path
			}
			if host, _ := v["host"].(string); host != "" {
				opts["headers"] = map[string]any{"Host": host}
			}
			p["ws-opts"] = opts
		case "grpc":
			if sn, _ := v["path"].(string); sn != "" {
				p["grpc-opts"] = map[string]any{"grpc-service-name": sn}
			}
		}
	}
	return p, nil
}

func parseSsURI(uri string) (map[string]any, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	name := u.Fragment
	var method, password, server string
	var port int

	if u.User != nil && u.Hostname() != "" {
		userInfo := u.User.Username()
		decoded, err := base64.RawURLEncoding.DecodeString(userInfo)
		if err != nil {
			decoded, _ = base64.StdEncoding.DecodeString(userInfo)
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) == 2 {
			method = parts[0]
			password = parts[1]
		} else {
			method = userInfo
			password, _ = u.User.Password()
		}
		server = u.Hostname()
		port, _ = strconv.Atoi(u.Port())
	} else {
		raw := strings.TrimPrefix(uri, "ss://")
		if idx := strings.Index(raw, "#"); idx != -1 {
			name = raw[idx+1:]
			raw = raw[:idx]
		}
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			decoded, _ = base64.StdEncoding.DecodeString(raw)
		}
		parsed, err := url.Parse("ss://" + string(decoded))
		if err != nil {
			return nil, err
		}
		method = parsed.User.Username()
		password, _ = parsed.User.Password()
		server = parsed.Hostname()
		port, _ = strconv.Atoi(parsed.Port())
	}
	if name == "" {
		name = server
	}
	return map[string]any{
		"name":     name,
		"type":     "ss",
		"server":   server,
		"port":     port,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}, nil
}

func parseTrojanURI(uri string) (map[string]any, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(u.Port())
	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}
	p := map[string]any{
		"name":     name,
		"type":     "trojan",
		"server":   u.Hostname(),
		"port":     port,
		"password": u.User.Username(),
		"udp":      true,
	}
	q := u.Query()
	if sni := q.Get("sni"); sni != "" {
		p["sni"] = sni
	}
	if q.Get("security") != "none" {
		p["tls"] = true
	}
	return p, nil
}

func proxiesToNodes(subID uint, proxies []map[string]any) []model.Node {
	seen := make(map[string]bool)
	var nodes []model.Node
	for _, p := range proxies {
		name, _ := p["name"].(string)
		typ, _ := p["type"].(string)
		server, _ := p["server"].(string)
		port := toInt(p["port"])
		if name == "" || server == "" {
			continue
		}
		key := fmt.Sprintf("%s:%s:%d", typ, server, port)
		if seen[key] {
			continue
		}
		seen[key] = true
		raw, _ := json.Marshal(p)
		nodes = append(nodes, model.Node{
			SubscriptionID: subID,
			Name:           name,
			Type:           typ,
			Server:         server,
			Port:           port,
			RawConfig:      string(raw),
			Enabled:        true,
		})
	}
	return nodes
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}
