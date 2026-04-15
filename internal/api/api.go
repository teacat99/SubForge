package api

import (
	"github.com/teacat99/SubForge/internal/generator"
	"github.com/teacat99/SubForge/internal/model"
	"github.com/teacat99/SubForge/internal/rule"
	"github.com/teacat99/SubForge/internal/store"
	"github.com/teacat99/SubForge/internal/subscription"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

type GinContext = gin.Context

type Handler struct {
	store        *store.Store
	sessionToken string
	mu           sync.Mutex
}

func NewRouter(s *store.Store) *gin.Engine {
	h := &Handler{store: s}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.POST("/api/login", h.login)
	r.GET("/sub", h.generateConfig)

	api := r.Group("/api")
	api.Use(h.authMiddleware())
	{
		api.GET("/me", h.me)
		api.POST("/logout", h.logout)
		api.POST("/password", h.changePassword)
		api.GET("/health", h.health)

		api.GET("/subscriptions", h.listSubscriptions)
		api.POST("/subscriptions", h.createSubscription)
		api.POST("/subscriptions/reorder", h.reorderSubscriptions)
		api.PATCH("/subscriptions/:id", h.updateSubscription)
		api.DELETE("/subscriptions/:id", h.deleteSubscription)
		api.POST("/subscriptions/:id/fetch", h.fetchSubscription)
		api.POST("/subscriptions/:id/import", h.importSubscription)
		api.GET("/subscriptions/:id/export", h.exportSubscription)

		api.GET("/settings/alias-presets", h.getAliasPresets)
		api.PUT("/settings/alias-presets", h.setAliasPresets)
		api.POST("/nodes/apply-alias-presets", h.applyAliasPresets)

		api.GET("/nodes", h.listNodes)
		api.POST("/nodes", h.createNode)
		api.PATCH("/nodes/:id", h.toggleNode)
		api.PATCH("/nodes/:id/alias", h.updateNodeAlias)
		api.PATCH("/nodes/:id/edit", h.editNode)
		api.GET("/nodes/:id/raw", h.getNodeRaw)
		api.DELETE("/nodes/:id", h.deleteNode)

		api.GET("/services", h.listServices)
		api.POST("/services", h.createService)
		api.POST("/services/reorder", h.reorderServices)
		api.PATCH("/services/:id", h.updateService)
		api.DELETE("/services/:id", h.deleteService)
		api.POST("/services/:id/fetch", h.fetchServiceRules)
		api.GET("/services/:id/rules", h.previewServiceRules)

		api.GET("/dns-presets", h.listDnsPresets)
		api.POST("/dns-presets", h.createDnsPreset)
		api.PATCH("/dns-presets/:id", h.updateDnsPreset)
		api.DELETE("/dns-presets/:id", h.deleteDnsPreset)

		api.GET("/profiles", h.listProfiles)
		api.POST("/profiles", h.createProfile)
		api.POST("/profiles/reorder", h.reorderProfiles)
		api.PATCH("/profiles/:id", h.updateProfile)
		api.DELETE("/profiles/:id", h.deleteProfile)
		api.GET("/profiles/:id/nodes", h.listProfileNodes)
		api.PATCH("/profiles/:id/nodes", h.updateProfileNodes)
		api.GET("/profiles/:id/services", h.listProfileServices)
		api.PATCH("/profiles/:id/services/:sgid/toggle", h.toggleProfileService)
		api.PATCH("/profiles/:id/services/:sgid/proxy", h.updateProfileServiceProxy)
		api.POST("/profiles/:id/services/reorder", h.reorderProfileServices)
		api.POST("/profiles/:id/services/reset-order", h.resetProfileServiceOrder)
		api.GET("/profiles/:id/preview", h.previewProfileConfig)
	}

	return r
}

// ── Auth ──

func (h *Handler) login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	user, err := h.store.GetAdmin(req.Username)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}
	token := generateToken()
	h.mu.Lock()
	h.sessionToken = token
	h.mu.Unlock()
	c.SetCookie("session", token, 86400, "/", "", false, true)
	c.JSON(200, gin.H{"ok": true, "username": user.Username})
}

func (h *Handler) logout(c *gin.Context) {
	h.mu.Lock()
	h.sessionToken = ""
	h.mu.Unlock()
	c.SetCookie("session", "", -1, "/", "", false, true)
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) me(c *gin.Context) {
	admin, err := h.store.GetFirstAdmin()
	if err != nil {
		c.JSON(200, gin.H{"username": "admin"})
		return
	}
	c.JSON(200, gin.H{"username": admin.Username})
}

func (h *Handler) changePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
		NewUsername  string `json:"new_username"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.NewPassword == "" && req.NewUsername == "" {
		c.JSON(400, gin.H{"error": "no changes"})
		return
	}
	admin, err := h.store.GetFirstAdmin()
	if err != nil {
		c.JSON(500, gin.H{"error": "admin not found"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.Password), []byte(req.OldPassword)); err != nil {
		c.JSON(401, gin.H{"error": "wrong old password"})
		return
	}
	if req.NewUsername != "" {
		if err := h.store.UpdateAdminUsername(admin.ID, req.NewUsername); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	if req.NewPassword != "" {
		if err := h.store.UpdateAdminPassword(admin.ID, req.NewPassword); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		h.mu.Lock()
		h.sessionToken = ""
		h.mu.Unlock()
		c.SetCookie("session", "", -1, "/", "", false, true)
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("session")
		if err != nil || cookie == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		h.mu.Lock()
		valid := cookie == h.sessionToken && h.sessionToken != ""
		h.mu.Unlock()
		if !valid {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ── Subscription ──

func (h *Handler) listSubscriptions(c *gin.Context) {
	subs, err := h.store.ListSubscriptions()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, subs)
}

func (h *Handler) reorderSubscriptions(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.ReorderSubscriptions(body.IDs); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) createSubscription(c *gin.Context) {
	var sub model.Subscription
	if err := c.ShouldBindJSON(&sub); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if sub.IntervalSec == 0 {
		sub.IntervalSec = 86400
	}
	if sub.URL == "" {
		sub.AutoRefresh = false
	} else {
		sub.AutoRefresh = true
	}
	if sub.ExcludeKeywords == "" {
		sub.ExcludeKeywords = "剩余流量"
	}
	if err := h.store.CreateSubscription(&sub); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, sub)
}

func (h *Handler) updateSubscription(c *gin.Context) {
	id := parseID(c)
	sub, err := h.store.GetSubscription(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if err := c.ShouldBindJSON(sub); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	sub.ID = id
	if sub.URL == "" {
		sub.AutoRefresh = false
	}
	if err := h.store.UpdateSubscription(sub); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, sub)
}

func (h *Handler) deleteSubscription(c *gin.Context) {
	id := parseID(c)
	if err := h.store.DeleteSubscription(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) fetchSubscription(c *gin.Context) {
	id := parseID(c)
	sub, err := h.store.GetSubscription(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	nodes, info, err := subscription.Fetch(sub)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	nodes = filterExcludedNodes(nodes, sub.ExcludeKeywords)
	h.applyAliasPresetsToNodes(nodes)
	if err := h.store.ReplaceNodes(id, nodes); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	h.store.UpdateSubscriptionFetched(id)
	if info != nil {
		used := info.Upload + info.Download
		expiry := ""
		if info.Expire > 0 {
			expiry = time.Unix(info.Expire, 0).Format("2006-01-02")
		}
		h.store.UpdateSubscriptionTraffic(id, used, info.Total, expiry)
	}
	c.JSON(200, gin.H{"ok": true, "count": len(nodes)})
}

func (h *Handler) importSubscription(c *gin.Context) {
	id := parseID(c)
	sub, err := h.store.GetSubscription(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Content == "" {
		c.JSON(400, gin.H{"error": "content required"})
		return
	}
	nodes, err := subscription.ParseContent(sub.ID, []byte(body.Content))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	nodes = filterExcludedNodes(nodes, sub.ExcludeKeywords)
	h.applyAliasPresetsToNodes(nodes)
	if err := h.store.ReplaceNodes(id, nodes); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	h.store.UpdateSubscriptionFetched(id)
	c.JSON(200, gin.H{"ok": true, "count": len(nodes)})
}

func (h *Handler) exportSubscription(c *gin.Context) {
	id := parseID(c)
	nodes, err := h.store.ListNodesBySubscription(id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	var proxies []map[string]any
	for _, n := range nodes {
		var p map[string]any
		if err := json.Unmarshal([]byte(n.RawConfig), &p); err == nil {
			proxies = append(proxies, p)
		}
	}
	type proxyDoc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	data, _ := yaml.Marshal(proxyDoc{Proxies: proxies})
	c.JSON(200, gin.H{"content": string(data)})
}

// ── Alias Presets ──

func (h *Handler) getAliasPresets(c *gin.Context) {
	val, _ := h.store.GetSetting("alias_presets")
	c.JSON(200, gin.H{"presets": val})
}

func (h *Handler) setAliasPresets(c *gin.Context) {
	var body struct {
		Presets string `json:"presets"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.SetSetting("alias_presets", body.Presets); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func parseAliasPresets(raw string) []aliasPreset {
	var presets []aliasPreset
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		keyword := strings.TrimSpace(parts[0])
		alias := strings.TrimSpace(parts[1])
		if keyword != "" && alias != "" {
			presets = append(presets, aliasPreset{keyword: keyword, alias: alias})
		}
	}
	return presets
}

type aliasPreset struct {
	keyword string
	alias   string
}

func (h *Handler) applyAliasPresetsToNodes(nodes []model.Node) {
	raw, err := h.store.GetSetting("alias_presets")
	if err != nil || strings.TrimSpace(raw) == "" {
		return
	}
	presets := parseAliasPresets(raw)
	if len(presets) == 0 {
		return
	}
	for i := range nodes {
		if nodes[i].Alias != "" {
			continue
		}
		for _, p := range presets {
			if strings.Contains(nodes[i].Name, p.keyword) {
				nodes[i].Alias = p.alias
				break
			}
		}
	}
}

func (h *Handler) applyAliasPresets(c *gin.Context) {
	raw, _ := h.store.GetSetting("alias_presets")
	presets := parseAliasPresets(raw)
	if len(presets) == 0 {
		c.JSON(200, gin.H{"ok": true, "updated": 0})
		return
	}
	nodes, err := h.store.ListNodes()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	updated := 0
	for _, n := range nodes {
		if n.Alias != "" {
			continue
		}
		for _, p := range presets {
			if strings.Contains(n.Name, p.keyword) {
				h.store.UpdateNodeAlias(n.ID, p.alias)
				updated++
				break
			}
		}
	}
	c.JSON(200, gin.H{"ok": true, "updated": updated})
}

// ── Node ──

type nodeView struct {
	model.Node
	SubName       string `json:"sub_name"`
	AppendSubName bool   `json:"append_sub_name"`
	AppendTraffic bool   `json:"append_traffic"`
	TrafficUsed   int64  `json:"traffic_used"`
	TrafficTotal  int64  `json:"traffic_total"`
}

func (h *Handler) listNodes(c *gin.Context) {
	nodes, err := h.store.ListNodes()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	subs, _ := h.store.ListSubscriptions()
	subMap := make(map[uint]*model.Subscription)
	for i := range subs {
		subMap[subs[i].ID] = &subs[i]
	}

	result := make([]nodeView, len(nodes))
	for i, n := range nodes {
		nv := nodeView{Node: n}
		if n.SubscriptionID == 0 {
			nv.SubName = "本地"
		} else if sub, ok := subMap[n.SubscriptionID]; ok {
			nv.SubName = sub.Name
			nv.AppendSubName = sub.AppendSubName
			nv.AppendTraffic = sub.AppendTraffic
			nv.TrafficUsed = sub.TrafficUsed
			nv.TrafficTotal = sub.TrafficTotal
		}
		result[i] = nv
	}
	c.JSON(200, result)
}

func (h *Handler) toggleNode(c *gin.Context) {
	id := parseID(c)
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.ToggleNode(id, body.Enabled); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) updateNodeAlias(c *gin.Context) {
	id := parseID(c)
	var body struct {
		Alias string `json:"alias"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateNodeAlias(id, body.Alias); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) createNode(c *gin.Context) {
	var body struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Server    string `json:"server"`
		Port      int    `json:"port"`
		RawConfig string `json:"raw_config"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.RawConfig == "" {
		c.JSON(400, gin.H{"error": "raw_config required"})
		return
	}
	node := model.Node{
		SubscriptionID: 0,
		Name:           body.Name,
		Type:           body.Type,
		Server:         body.Server,
		Port:           body.Port,
		RawConfig:      body.RawConfig,
		Enabled:        true,
	}
	if err := h.store.CreateNode(&node); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"ok": true, "id": node.ID})
}

func (h *Handler) getNodeRaw(c *gin.Context) {
	id := parseID(c)
	node, err := h.store.GetNode(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, gin.H{
		"id":         node.ID,
		"name":       node.Name,
		"type":       node.Type,
		"server":     node.Server,
		"port":       node.Port,
		"raw_config": node.RawConfig,
		"subscription_id": node.SubscriptionID,
	})
}

func (h *Handler) editNode(c *gin.Context) {
	id := parseID(c)
	node, err := h.store.GetNode(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var body struct {
		RawConfig string `json:"raw_config"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	node.RawConfig = body.RawConfig
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body.RawConfig), &parsed); err == nil {
		if n, ok := parsed["name"].(string); ok && n != "" {
			node.Name = n
		}
		if t, ok := parsed["type"].(string); ok && t != "" {
			node.Type = t
		}
		if s, ok := parsed["server"].(string); ok && s != "" {
			node.Server = s
		}
		if p, ok := parsed["port"]; ok {
			switch v := p.(type) {
			case float64:
				node.Port = int(v)
			}
		}
	}
	if err := h.store.UpdateNode(node); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) deleteNode(c *gin.Context) {
	id := parseID(c)
	if err := h.store.DeleteNode(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// ── ServiceGroup ──

type serviceView struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	Icon         string     `json:"icon"`
	RuleType     string     `json:"rule_type"`
	RuleURL      string     `json:"rule_url"`
	DefaultProxy string     `json:"default_proxy"`
	SortOrder    int        `json:"sort_order"`
	RuleCount    int        `json:"rule_count"`
	Enabled      bool       `json:"enabled"`
	AutoRefresh  bool       `json:"auto_refresh"`
	IntervalSec  int        `json:"interval_sec"`
	LastFetched  *time.Time `json:"last_fetched"`
	Builtin      bool       `json:"builtin"`
}

func (h *Handler) listServices(c *gin.Context) {
	groups, err := h.store.ListServiceGroups()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	result := make([]serviceView, len(groups))
	for i, g := range groups {
		result[i] = serviceView{
			ID: g.ID, Name: g.Name, Icon: g.Icon,
			RuleType: g.RuleType, RuleURL: g.RuleURL,
			DefaultProxy: g.DefaultProxy, SortOrder: g.SortOrder,
			RuleCount: g.RuleCount, Enabled: g.Enabled,
			AutoRefresh: g.AutoRefresh,
			IntervalSec: g.IntervalSec, LastFetched: g.LastFetched,
			Builtin: g.Builtin,
		}
	}
	c.JSON(200, result)
}

func (h *Handler) reorderServices(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.ReorderServiceGroups(body.IDs); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) createService(c *gin.Context) {
	var body struct {
		Name         string `json:"name"`
		Icon         string `json:"icon"`
		RuleType     string `json:"rule_type"`
		RuleURL      string `json:"rule_url"`
		CachedRules  string `json:"cached_rules"`
		DefaultProxy string `json:"default_proxy"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	g := model.ServiceGroup{
		Name:         body.Name,
		Icon:         body.Icon,
		RuleType:     body.RuleType,
		RuleURL:      body.RuleURL,
		DefaultProxy: body.DefaultProxy,
		CachedRules:  body.CachedRules,
		Enabled:      true,
	}
	if body.Enabled != nil {
		g.Enabled = *body.Enabled
	}
	if g.DefaultProxy == "" {
		g.DefaultProxy = "自动选择"
	}
	if g.IntervalSec == 0 {
		g.IntervalSec = 86400
	}
	if g.RuleType == "" {
		g.RuleType = "remote"
	}
	g.AutoRefresh = g.RuleType == "remote"
	if err := h.store.CreateServiceGroup(&g); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if g.RuleType == "remote" && g.RuleURL != "" {
		go func() {
			if err := rule.FetchRules(&g); err == nil {
				h.store.UpdateServiceGroup(&g)
			}
		}()
	} else if g.RuleType == "local" {
		rule.FetchRules(&g)
		h.store.UpdateServiceGroup(&g)
	}
	c.JSON(201, gin.H{"ok": true, "id": g.ID})
}

func (h *Handler) updateService(c *gin.Context) {
	id := parseID(c)
	g, err := h.store.GetServiceGroup(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var body struct {
		Name         string `json:"name"`
		Icon         string `json:"icon"`
		RuleType     string `json:"rule_type"`
		RuleURL      string `json:"rule_url"`
		CachedRules  string `json:"cached_rules"`
		DefaultProxy string `json:"default_proxy"`
		AutoRefresh  *bool  `json:"auto_refresh"`
		Enabled      *bool  `json:"enabled"`
		IntervalSec  int    `json:"interval_sec"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Name != "" {
		g.Name = body.Name
	}
	if body.Icon != "" {
		g.Icon = body.Icon
	}
	if body.RuleType != "" {
		g.RuleType = body.RuleType
	}
	if body.RuleURL != "" {
		g.RuleURL = body.RuleURL
	}
	if body.DefaultProxy != "" {
		g.DefaultProxy = body.DefaultProxy
	}
	if body.AutoRefresh != nil {
		g.AutoRefresh = *body.AutoRefresh
	}
	if body.Enabled != nil {
		g.Enabled = *body.Enabled
	}
	if body.IntervalSec > 0 {
		g.IntervalSec = body.IntervalSec
	}
	if g.RuleType == "local" && body.CachedRules != "" {
		g.CachedRules = body.CachedRules
		rule.FetchRules(g)
	}
	if err := h.store.UpdateServiceGroup(g); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) deleteService(c *gin.Context) {
	id := parseID(c)
	if err := h.store.DeleteServiceGroup(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) fetchServiceRules(c *gin.Context) {
	id := parseID(c)
	g, err := h.store.GetServiceGroup(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if g.RuleType == "local" {
		c.JSON(400, gin.H{"error": "local rules cannot be fetched"})
		return
	}
	if err := rule.FetchRules(g); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateServiceGroup(g); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "rule_count": g.RuleCount})
}

func (h *Handler) previewServiceRules(c *gin.Context) {
	id := parseID(c)
	g, err := h.store.GetServiceGroup(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	parsed := rule.ParseCachedRules(g.CachedRules, g.Name)
	c.JSON(200, gin.H{
		"rules":       parsed,
		"cached_rules": g.CachedRules,
		"rule_count":  g.RuleCount,
	})
}

// ── DNS Preset ──

func (h *Handler) listDnsPresets(c *gin.Context) {
	presets, err := h.store.ListDnsPresets()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, presets)
}

func (h *Handler) createDnsPreset(c *gin.Context) {
	var p model.DnsPreset
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.CreateDnsPreset(&p); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, p)
}

func (h *Handler) updateDnsPreset(c *gin.Context) {
	id := parseID(c)
	p, err := h.store.GetDnsPreset(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var body struct {
		Name   string `json:"name"`
		Config string `json:"config"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Name != "" {
		p.Name = body.Name
	}
	if body.Config != "" {
		p.Config = body.Config
	}
	if err := h.store.UpdateDnsPreset(p); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, p)
}

func (h *Handler) deleteDnsPreset(c *gin.Context) {
	id := parseID(c)
	if err := h.store.DeleteDnsPreset(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// ── Profile ──

func (h *Handler) listProfiles(c *gin.Context) {
	profiles, err := h.store.ListProfiles()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, profiles)
}

func (h *Handler) createProfile(c *gin.Context) {
	var p model.UserProfile
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	p.Token = generateToken()
	p.Enabled = true
	p.CatchAll = false
	p.GeoipCN = true
	if err := h.store.CreateProfile(&p); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, p)
}

func (h *Handler) reorderProfiles(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.ReorderProfiles(body.IDs); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) updateProfile(c *gin.Context) {
	id := parseID(c)
	p, err := h.store.GetProfile(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	var body struct {
		Name        *string    `json:"name"`
		Enabled     *bool      `json:"enabled"`
		ExpiresAt   *time.Time `json:"expires_at"`
		CatchAll    *bool      `json:"catch_all"`
		GeoipCN     *bool      `json:"geoip_cn"`
		DnsPresetID *uint      `json:"dns_preset_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if body.Name != nil {
		p.Name = *body.Name
	}
	if body.Enabled != nil {
		p.Enabled = *body.Enabled
	}
	if body.ExpiresAt != nil {
		p.ExpiresAt = body.ExpiresAt
	}
	if body.CatchAll != nil {
		p.CatchAll = *body.CatchAll
	}
	if body.GeoipCN != nil {
		p.GeoipCN = *body.GeoipCN
	}
	if body.DnsPresetID != nil {
		p.DnsPresetID = *body.DnsPresetID
	}
	p.ID = id
	if err := h.store.UpdateProfile(p); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, p)
}

func (h *Handler) deleteProfile(c *gin.Context) {
	id := parseID(c)
	if err := h.store.DeleteProfile(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// ── Profile Nodes ──

func (h *Handler) listProfileNodes(c *gin.Context) {
	profileID := parseID(c)
	pn, err := h.store.ListProfileNodes(profileID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	pnMap := make(map[uint]bool)
	for _, p := range pn {
		pnMap[p.NodeID] = p.Enabled
	}

	nodes, _ := h.store.ListNodes()
	subs, _ := h.store.ListSubscriptions()
	subMap := make(map[uint]*model.Subscription)
	for i := range subs {
		subMap[subs[i].ID] = &subs[i]
	}

	type profileNodeView struct {
		NodeID        uint   `json:"node_id"`
		Name          string `json:"name"`
		Alias         string `json:"alias"`
		SubName       string `json:"sub_name"`
		Enabled       bool   `json:"enabled"`
		AppendSubName bool   `json:"append_sub_name"`
		AppendTraffic bool   `json:"append_traffic"`
		TrafficUsed   int64  `json:"traffic_used"`
	}
	result := make([]profileNodeView, len(nodes))
	for i, n := range nodes {
		enabled := true
		if e, ok := pnMap[n.ID]; ok {
			enabled = e
		}
		pnv := profileNodeView{
			NodeID:  n.ID,
			Name:    n.Name,
			Alias:   n.Alias,
			Enabled: enabled,
		}
		if n.SubscriptionID == 0 {
			pnv.SubName = "本地"
		} else if sub, ok := subMap[n.SubscriptionID]; ok {
			pnv.SubName = sub.Name
			pnv.AppendSubName = sub.AppendSubName
			pnv.AppendTraffic = sub.AppendTraffic
			pnv.TrafficUsed = sub.TrafficUsed
		}
		result[i] = pnv
	}
	c.JSON(200, result)
}

func (h *Handler) updateProfileNodes(c *gin.Context) {
	profileID := parseID(c)
	var body struct {
		Nodes map[string]bool `json:"nodes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	states := make(map[uint]bool)
	for k, v := range body.Nodes {
		id, _ := strconv.ParseUint(k, 10, 64)
		if id > 0 {
			states[uint(id)] = v
		}
	}
	if err := h.store.SetProfileNodes(profileID, states); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// ── Profile Services ──

func (h *Handler) listProfileServices(c *gin.Context) {
	profileID := parseID(c)
	ps, err := h.store.ListProfileServices(profileID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	groups, _ := h.store.ListServiceGroups()
	gMap := make(map[uint]*model.ServiceGroup)
	for i := range groups {
		gMap[groups[i].ID] = &groups[i]
	}

	existingMap := make(map[uint]bool)
	for _, p := range ps {
		existingMap[p.ServiceGroupID] = true
	}
	for _, g := range groups {
		if !existingMap[g.ID] {
			newPS := model.ProfileService{
				ProfileID:      profileID,
				ServiceGroupID: g.ID,
				Enabled:        false,
				SortOrder:      g.SortOrder,
			}
			h.store.CreateProfileService(&newPS)
			ps = append(ps, newPS)
		}
	}

	type profileSvcView struct {
		ServiceGroupID    uint   `json:"service_group_id"`
		Name              string `json:"name"`
		Icon              string `json:"icon"`
		DefaultProxy      string `json:"default_proxy"`
		DefaultProxyOverride string `json:"default_proxy_override"`
		RuleCount         int    `json:"rule_count"`
		Enabled           bool   `json:"enabled"`
		SortOrder         int    `json:"sort_order"`
	}
	result := make([]profileSvcView, 0)
	for _, p := range ps {
		g, ok := gMap[p.ServiceGroupID]
		if !ok {
			continue
		}
		result = append(result, profileSvcView{
			ServiceGroupID:    p.ServiceGroupID,
			Name:              g.Name,
			Icon:              g.Icon,
			DefaultProxy:      g.DefaultProxy,
			DefaultProxyOverride: p.DefaultProxy,
			RuleCount:         g.RuleCount,
			Enabled:           p.Enabled,
			SortOrder:         p.SortOrder,
		})
	}
	c.JSON(200, result)
}

func (h *Handler) toggleProfileService(c *gin.Context) {
	profileID := parseID(c)
	sgID, _ := strconv.ParseUint(c.Param("sgid"), 10, 64)
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.ToggleProfileService(profileID, uint(sgID), body.Enabled); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) updateProfileServiceProxy(c *gin.Context) {
	profileID := parseID(c)
	sgID, _ := strconv.ParseUint(c.Param("sgid"), 10, 64)
	var body struct {
		DefaultProxy string `json:"default_proxy"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateProfileServiceProxy(profileID, uint(sgID), body.DefaultProxy); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) reorderProfileServices(c *gin.Context) {
	profileID := parseID(c)
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.ReorderProfileServices(profileID, body.IDs); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handler) resetProfileServiceOrder(c *gin.Context) {
	profileID := parseID(c)
	if err := h.store.ResetProfileServiceOrder(profileID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// ── Config Generation ──

func (h *Handler) buildGenerateOptions(profile *model.UserProfile) (*generator.Options, error) {
	ps, _ := h.store.ListProfileServices(profile.ID)
	var enabledSvcIDs []uint
	sortMap := make(map[uint]int)
	proxyOverride := make(map[uint]string)
	for _, p := range ps {
		if p.Enabled {
			enabledSvcIDs = append(enabledSvcIDs, p.ServiceGroupID)
			sortMap[p.ServiceGroupID] = p.SortOrder
			if p.DefaultProxy != "" {
				proxyOverride[p.ServiceGroupID] = p.DefaultProxy
			}
		}
	}

	groups, _ := h.store.GetServiceGroupsByIDs(enabledSvcIDs)
	for i := range groups {
		if so, ok := sortMap[groups[i].ID]; ok {
			groups[i].SortOrder = so
		}
	}
	sortByOrder := func(i, j int) bool {
		return groups[i].SortOrder < groups[j].SortOrder
	}
	sortSlice(groups, sortByOrder)

	nodeIDs, _ := h.store.GetEnabledNodeIDsForProfile(profile.ID)
	nodes, _ := h.store.GetNodesByIDs(nodeIDs)
	if len(nodes) == 0 {
		allNodes, _ := h.store.ListEnabledNodes()
		nodes = allNodes
	}

	subs, _ := h.store.ListSubscriptions()

	subMap := make(map[uint]*model.Subscription)
	for i := range subs {
		subMap[subs[i].ID] = &subs[i]
	}
	nodeNameSet := make(map[string]bool)
	for _, n := range nodes {
		nodeNameSet[generator.ComputeDisplayName(n, subMap)] = true
	}

	builtins := map[string]bool{"自动选择": true, "DIRECT": true, "REJECT": true, "节点选择": true}
	for i := range groups {
		if dp, ok := proxyOverride[groups[i].ID]; ok {
			if builtins[dp] || nodeNameSet[dp] {
				groups[i].DefaultProxy = dp
			}
		}
	}

	var dnsConfig string
	if profile.DnsPresetID > 0 {
		if preset, err := h.store.GetDnsPreset(profile.DnsPresetID); err == nil {
			dnsConfig = preset.Config
		}
	}

	return &generator.Options{
		Nodes:         nodes,
		Subscriptions: subs,
		ServiceGroups: groups,
		CatchAll:      profile.CatchAll,
		GeoipCN:       profile.GeoipCN,
		DnsConfig:     dnsConfig,
	}, nil
}

func sortSlice(groups []model.ServiceGroup, less func(i, j int) bool) {
	for i := 1; i < len(groups); i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			groups[j], groups[j-1] = groups[j-1], groups[j]
		}
	}
}

func (h *Handler) generateConfig(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.String(403, "missing token")
		return
	}
	profile, err := h.store.GetProfileByToken(token)
	if err != nil {
		c.String(403, "invalid token")
		return
	}
	if !profile.Enabled {
		c.String(403, "profile disabled")
		return
	}
	if profile.ExpiresAt != nil && profile.ExpiresAt.Before(time.Now()) {
		c.String(403, "profile expired")
		return
	}

	opts, err := h.buildGenerateOptions(profile)
	if err != nil {
		c.String(500, "failed to build options: %v", err)
		return
	}
	if len(opts.Nodes) == 0 {
		c.String(500, "no nodes available")
		return
	}

	data, err := generator.Generate(*opts)
	if err != nil {
		c.String(500, "config generation failed: %v", err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=config.yaml")
	c.Data(200, "text/yaml; charset=utf-8", data)
}

func (h *Handler) previewProfileConfig(c *gin.Context) {
	id := parseID(c)
	profile, err := h.store.GetProfile(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	opts, err := h.buildGenerateOptions(profile)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if len(opts.Nodes) == 0 {
		c.JSON(200, gin.H{"config": "# No nodes available"})
		return
	}
	data, err := generator.Generate(*opts)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"config": string(data)})
}

// ── Helpers ──

func filterExcludedNodes(nodes []model.Node, keywords string) []model.Node {
	if strings.TrimSpace(keywords) == "" {
		return nodes
	}
	parts := strings.Split(keywords, ",")
	var kw []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			kw = append(kw, p)
		}
	}
	if len(kw) == 0 {
		return nodes
	}
	var result []model.Node
	for _, n := range nodes {
		excluded := false
		for _, k := range kw {
			if strings.Contains(n.Name, k) {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, n)
		}
	}
	return result
}

func parseID(c *gin.Context) uint {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id)
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
