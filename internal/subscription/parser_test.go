package subscription

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teacat99/SubForge/internal/model"
)

func TestParseContentClashYAMLProxies(t *testing.T) {
	nodes, err := ParseContent(7, []byte(`
proxies:
  - name: direct-node
    type: vless
    server: direct.example.com
    port: 443
    uuid: test-uuid
`))
	if err != nil {
		t.Fatalf("ParseContent returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].SubscriptionID != 7 || nodes[0].Name != "direct-node" || nodes[0].Server != "direct.example.com" {
		t.Fatalf("unexpected node: %+v", nodes[0])
	}
}

func TestParseContentMihomoProfileProxyProviders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider.yaml" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`
proxies:
  - name: provider-node
    type: trojan
    server: provider.example.com
    port: 443
    password: secret
`))
	}))
	defer server.Close()

	profile := `
mixed-port: 7890
proxy-providers:
  suda:
    type: http
    url: "` + server.URL + `/provider.yaml"
    path: ./providers/suda.yaml
    interval: 86400
proxy-groups:
  - name: 节点选择
    type: select
    use:
      - suda
rules:
  - MATCH,节点选择
`

	nodes, err := ParseContent(9, []byte(profile))
	if err != nil {
		t.Fatalf("ParseContent returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].SubscriptionID != 9 || nodes[0].Name != "provider-node" || nodes[0].Type != "trojan" {
		t.Fatalf("unexpected node: %+v", nodes[0])
	}
}

func TestFetchMihomoProfileProxyProviders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/profile.yaml":
			_, _ = w.Write([]byte(`
proxy-providers:
  first:
    type: http
    url: "` + "http://" + r.Host + `/provider.yaml"
`))
		case "/provider.yaml":
			_, _ = w.Write([]byte(`
proxies:
  - name: fetched-node
    type: ss
    server: fetched.example.com
    port: 8388
    cipher: aes-128-gcm
    password: secret
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	nodes, _, err := Fetch(&model.Subscription{ID: 11, URL: server.URL + "/profile.yaml"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if !strings.Contains(nodes[0].RawConfig, "fetched-node") {
		t.Fatalf("raw config does not contain fetched node name: %s", nodes[0].RawConfig)
	}
}
