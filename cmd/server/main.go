package main

import (
	"github.com/teacat99/SubForge/internal/api"
	"github.com/teacat99/SubForge/internal/model"
	"github.com/teacat99/SubForge/internal/rule"
	"github.com/teacat99/SubForge/internal/store"
	"github.com/teacat99/SubForge/internal/subscription"
	"github.com/teacat99/SubForge/web"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const defaultDnsYAML = `enable: true
ipv6: false
listen: :53
prefer-h3: false
respect-rules: true
enhanced-mode: fake-ip
fake-ip-range: 28.0.0.1/8
fake-ip-filter:
  - "*.lan"
  - "*.localdomain"
  - "*.localhost"
  - "*.local"
  - "+.msftconnecttest.com"
  - "+.msftncsi.com"
  - "time.*.com"
  - "ntp.*.com"
  - "+.pool.ntp.org"
  - "time1.cloud.tencent.com"
  - "music.163.com"
  - "*.music.163.com"
  - "*.126.net"
  - "y.qq.com"
  - "*.y.qq.com"
  - "+.srv.nintendo.net"
  - "+.stun.playstation.net"
  - "+.battlenet.com.cn"
  - "lens.l.google.com"
  - "stun.l.google.com"
use-hosts: true
use-system-hosts: false
nameserver:
  - https://1.1.1.1/dns-query
  - https://dns.google/dns-query
default-nameserver:
  - tls://223.5.5.5
  - tls://119.29.29.29
proxy-server-nameserver:
  - https://doh.pub/dns-query
direct-nameserver:
  - https://doh.pub/dns-query
  - https://dns.alidns.com/dns-query
`

func main() {
	port := flag.Int("port", 8080, "server listen port")
	dbPath := flag.String("db", "", "SQLite database path")
	flag.Parse()

	if *dbPath == "" {
		exe, _ := os.Executable()
		dataDir := filepath.Join(filepath.Dir(exe), "data")
		os.MkdirAll(dataDir, 0755)
		*dbPath = filepath.Join(dataDir, "subforge.db")
	}

	s, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}

	if err := s.EnsureAdmin("admin", "passwd"); err != nil {
		log.Fatalf("failed to create admin: %v", err)
	}
	log.Printf("admin user ready (default: admin/passwd)")

	if err := s.SeedServiceGroups(rule.DefaultServiceGroups()); err != nil {
		log.Fatalf("failed to seed service groups: %v", err)
	}
	log.Printf("service groups seeded")

	s.SeedDnsPresets([]model.DnsPreset{
		{Name: "DNS 1.1.1.1", Config: defaultDnsYAML, Builtin: true},
	})
	log.Printf("DNS presets seeded")

	s.EnsureMigrate()
	cleanNodeNames(s)
	go initialRuleFetch(s)
	go autoFetch(s)

	router := api.NewRouter(s)

	webFS, _ := fs.Sub(web.FS, ".")
	router.NoRoute(func(c *api.GinContext) {
		data, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("SubForge server starting on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func initialRuleFetch(s *store.Store) {
	groups, err := s.ListServiceGroups()
	if err != nil {
		return
	}
	for i := range groups {
		g := &groups[i]
		if g.RuleType == "local" {
			if g.CachedRules != "" && g.RuleCount == 0 {
				if err := rule.FetchRules(g); err == nil {
					s.UpdateServiceGroup(g)
					log.Printf("[init-fetch] %s: %d local rules", g.Name, g.RuleCount)
				}
			}
			continue
		}
		if g.CachedRules != "" {
			continue
		}
		if g.RuleURL == "" {
			continue
		}
		if err := rule.FetchRules(g); err != nil {
			log.Printf("[init-fetch] %s failed: %v", g.Name, err)
			continue
		}
		s.UpdateServiceGroup(g)
		log.Printf("[init-fetch] %s: %d rules", g.Name, g.RuleCount)
	}
}

var trafficSuffixRe = regexp.MustCompile(`-\d+\.?\d*(B|KB|MB|GB|TB)$`)

func cleanNodeNames(s *store.Store) {
	nodes, err := s.ListNodes()
	if err != nil {
		return
	}
	subs, _ := s.ListSubscriptions()
	subNameMap := make(map[uint]string)
	for _, sub := range subs {
		subNameMap[sub.ID] = sub.Name
	}

	cleaned := 0
	for _, n := range nodes {
		name := n.Name
		name = trafficSuffixRe.ReplaceAllString(name, "")
		if subName, ok := subNameMap[n.SubscriptionID]; ok && subName != "" {
			name = strings.TrimSuffix(name, "-"+subName)
		}
		name = trafficSuffixRe.ReplaceAllString(name, "")
		if name != n.Name && name != "" {
			s.UpdateNodeName(n.ID, name)
			cleaned++
		}
	}
	if cleaned > 0 {
		log.Printf("[cleanup] cleaned %d node names with stale suffixes", cleaned)
	}
}

func autoFetch(s *store.Store) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		subs, err := s.ListSubscriptions()
		if err != nil {
			continue
		}
		for i := range subs {
			sub := &subs[i]
			if !sub.AutoRefresh {
				continue
			}
			if sub.LastFetched != nil && time.Since(*sub.LastFetched) < time.Duration(sub.IntervalSec)*time.Second {
				continue
			}
			nodes, info, err := subscription.Fetch(sub)
			if err != nil {
				log.Printf("[auto-fetch] %s failed: %v", sub.Name, err)
				continue
			}
			if err := s.ReplaceNodes(sub.ID, nodes); err != nil {
				continue
			}
			s.UpdateSubscriptionFetched(sub.ID)
			if info != nil {
				used := info.Upload + info.Download
				expiry := ""
				if info.Expire > 0 {
					expiry = time.Unix(info.Expire, 0).Format("2006-01-02")
				}
				s.UpdateSubscriptionTraffic(sub.ID, used, info.Total, expiry)
			}
			log.Printf("[auto-fetch] %s: %d nodes", sub.Name, len(nodes))
		}

		groups, err := s.ListServiceGroups()
		if err != nil {
			continue
		}
		for i := range groups {
			g := &groups[i]
			if !g.AutoRefresh || g.RuleType == "local" {
				continue
			}
			if g.LastFetched != nil && time.Since(*g.LastFetched) < time.Duration(g.IntervalSec)*time.Second {
				continue
			}
			if err := rule.FetchRules(g); err != nil {
				log.Printf("[auto-refresh] %s failed: %v", g.Name, err)
				continue
			}
			s.UpdateServiceGroup(g)
			log.Printf("[auto-refresh] %s: %d rules", g.Name, g.RuleCount)
		}
	}
}
