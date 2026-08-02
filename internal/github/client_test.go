package github

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestErrNoStableRelease 验证当仓库所有 release 均为 draft/prerelease 时，
// GetLatestRelease 返回的错误可被 errors.Is 识别为 ErrNoStableRelease。
func TestErrNoStableRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 全部为 prerelease/draft
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"tag_name":"v1.0.0-rc1","prerelease":true,"draft":false},
			{"tag_name":"v1.0.0-alpha","prerelease":true,"draft":false},
			{"tag_name":"v0.9.0","prerelease":false,"draft":true}
		]`))
	}))
	defer srv.Close()

	c := NewClient()
	// 注入 mock transport（apiClient 私有字段，同包可访问）
	c.apiClient = &http.Client{Transport: &http.Transport{}}
	// 直接用 server URL 替换 host，简单起见借助 RoundTripper 改写
	c.apiClient.Transport = &urlRewriter{srv.URL, http.DefaultTransport}

	_, err := c.GetLatestRelease("aristocratos", "bpytop")
	if err == nil {
		t.Fatal("期望返回错误，但得到 nil")
	}
	if !errors.Is(err, ErrNoStableRelease) {
		t.Fatalf("errors.Is 未能识别 ErrNoStableRelease，实际错误: %v", err)
	}
}

// urlRewriter 将发往 api.github.com 的请求改写为测试服务器地址
type urlRewriter struct {
	base string
	rt   http.RoundTripper
}

func (u *urlRewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = "http"
	r.URL.Host = u.base[len("http://"):]
	r.URL.Path = "/repos/aristocratos/bpytop/releases"
	return u.rt.RoundTrip(r)
}
