package rule

import "github.com/teacat99/SubForge/internal/model"

const cdnBase = "https://cdn.jsdelivr.net/gh/blackmatrix7/ios_rule_script@master/rule/Clash/"
const faviconBase = "https://favicon.im/"

var defaultServices = []struct {
	Name         string
	Icon         string
	URL          string
	RuleType     string
	CachedRules  string
	DefaultProxy string
}{
	{"Google", faviconBase + "www.google.com?larger=true", cdnBase + "Google/Google.yaml", "remote", "", "自动选择"},
	{"YouTube", faviconBase + "www.youtube.com?larger=true", cdnBase + "YouTube/YouTube.yaml", "remote", "", "自动选择"},
	{"YouTube Music", faviconBase + "music.youtube.com?larger=true", cdnBase + "YouTubeMusic/YouTubeMusic.yaml", "remote", "", "自动选择"},
	{"Gemini", faviconBase + "gemini.google.com?larger=true", cdnBase + "Gemini/Gemini.yaml", "remote", "", "自动选择"},
	{"Docker", faviconBase + "www.docker.com?larger=true", cdnBase + "Docker/Docker.yaml", "remote", "", "自动选择"},
	{"Twitter", faviconBase + "x.com", cdnBase + "Twitter/Twitter.yaml", "remote", "", "自动选择"},
	{"OpenAI", faviconBase + "www.openai.com?larger=true", cdnBase + "OpenAI/OpenAI.yaml", "remote", "", "自动选择"},
	{"Claude", faviconBase + "claude.ai", cdnBase + "Claude/Claude.yaml", "remote", "", "自动选择"},
	{"GitHub", faviconBase + "github.com?larger=true", cdnBase + "GitHub/GitHub.yaml", "remote", "", "自动选择"},
	{"Telegram", faviconBase + "web.telegram.org", cdnBase + "Telegram/Telegram.yaml", "remote", "", "自动选择"},
	{"Facebook", faviconBase + "www.facebook.com?larger=true", cdnBase + "Facebook/Facebook.yaml", "remote", "", "自动选择"},
	{"Instagram", faviconBase + "www.instagram.com?larger=true", cdnBase + "Instagram/Instagram.yaml", "remote", "", "自动选择"},
	{"Netflix", faviconBase + "www.netflix.com?larger=true", cdnBase + "Netflix/Netflix.yaml", "remote", "", "自动选择"},
	{"Disney+", faviconBase + "www.disney.com", cdnBase + "Disney/Disney.yaml", "remote", "", "自动选择"},
	{"Spotify", faviconBase + "www.spotify.com?larger=true", cdnBase + "Spotify/Spotify.yaml", "remote", "", "自动选择"},
	{"TikTok", faviconBase + "www.tiktok.com?larger=true", cdnBase + "TikTok/TikTok.yaml", "remote", "", "自动选择"},
	{"Cursor", faviconBase + "cursor.com?larger=true", "", "local", "payload:\n - DOMAIN-KEYWORD,cursor,Cursor\n", "自动选择"},
	{"Copilot", faviconBase + "copilot.microsoft.com?larger=true", cdnBase + "Copilot/Copilot.yaml", "remote", "", "自动选择"},
	{"Microsoft", faviconBase + "www.microsoft.com?larger=true", cdnBase + "Microsoft/Microsoft.yaml", "remote", "", "DIRECT"},
	{"Steam", faviconBase + "store.steampowered.com?larger=true", cdnBase + "Steam/Steam.yaml", "remote", "", "DIRECT"},
	{"BiliBili", faviconBase + "www.bilibili.com?larger=true", cdnBase + "BiliBili/BiliBili.yaml", "remote", "", "DIRECT"},
	{"Apple", faviconBase + "www.apple.com?larger=true", cdnBase + "Apple/Apple.yaml", "remote", "", "DIRECT"},
}

func DefaultServiceGroups() []model.ServiceGroup {
	groups := make([]model.ServiceGroup, len(defaultServices))
	for i, s := range defaultServices {
		ruleType := s.RuleType
		if ruleType == "" {
			ruleType = "remote"
		}
		groups[i] = model.ServiceGroup{
			Name:         s.Name,
			Icon:         s.Icon,
			RuleURL:      s.URL,
			RuleType:     ruleType,
			CachedRules:  s.CachedRules,
			DefaultProxy: s.DefaultProxy,
			SortOrder:    i + 1,
			Enabled:      true,
			AutoRefresh:  ruleType == "remote",
			IntervalSec:  86400,
			Builtin:      true,
		}
	}
	return groups
}
