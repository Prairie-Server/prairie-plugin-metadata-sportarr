package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prairie-server/prairie-plugin-metadata-sportarr/metadata"
)

func TestClientRetryAndErrors(t *testing.T) {
	if NewClient(0).limiter == nil {
		t.Fatal("default rate")
	}

	t.Run("rate limit then ok", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits++
			if hits == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			_ = json.NewEncoder(w).Encode(AgentSearchResponse{})
		}))
		defer srv.Close()
		c := NewClient(100)
		c.SetBaseURL(srv.URL)
		if _, err := c.Search(context.Background(), "x"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("http 400", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusBadRequest)
		}))
		defer srv.Close()
		c := NewClient(100)
		c.SetBaseURL(srv.URL)
		if _, err := c.GetSeries(context.Background(), "x"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("decode error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{bad`))
		}))
		defer srv.Close()
		c := NewClient(100)
		c.SetBaseURL(srv.URL)
		if _, err := c.GetSeasons(context.Background(), "x"); err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("canceled wait", func(t *testing.T) {
		c := NewClient(100)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := c.doGet(ctx, "/x", &struct{}{}); err == nil {
			t.Fatal("expected cancel")
		}
	})

	t.Run("invalid request url", func(t *testing.T) {
		c := NewClient(100)
		c.SetBaseURL("http://example.com/%zz")
		if err := c.doGet(context.Background(), "", &struct{}{}); err == nil {
			t.Fatal("expected request creation error")
		}
	})

	t.Run("request failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
		url := srv.URL
		srv.Close()

		c := NewClient(100)
		c.httpClient.Timeout = 50 * time.Millisecond
		c.SetBaseURL(url)
		if err := c.doGet(context.Background(), "/x", &struct{}{}); err == nil {
			t.Fatal("expected request failure")
		}
	})

	t.Run("exhausted 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()
		c := NewClient(1000)
		c.SetBaseURL(srv.URL)
		if _, err := c.GetSeasonEpisodes(context.Background(), "x", 1); err == nil {
			t.Fatal("expected exhausted retries")
		}
	})

	resp := &http.Response{Header: http.Header{}}
	if retryAfterOrDefault(resp, 0) != time.Second {
		t.Fatal("default")
	}
	resp.Header.Set("Retry-After", "5")
	if retryAfterOrDefault(resp, 0) != 5*time.Second {
		t.Fatal("header")
	}
}

func TestNewProviderAndErrorPaths(t *testing.T) {
	p := NewProvider("")
	if p == nil || p.Name() != "Sportarr" || p.ForTypes()[0] != "series" {
		t.Fatalf("%#v", p)
	}
	p = NewProvider("https://example.invalid")
	if p.client.baseURL != "https://example.invalid" {
		t.Fatalf("base = %q", p.client.baseURL)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusBadRequest)
	}))
	defer srv.Close()
	c := NewClient(100)
	c.SetBaseURL(srv.URL)
	p = NewProviderWithClient(c)

	if _, err := p.Search(context.Background(), metadata.SearchQuery{Title: "x"}); err == nil {
		t.Fatal("search title error")
	}
	if _, err := p.Search(context.Background(), metadata.SearchQuery{ProviderIDs: map[string]string{"sportarr": "x"}}); err == nil {
		t.Fatal("search id error")
	}
	if _, err := p.GetMetadata(context.Background(), metadata.MetadataRequest{ProviderIDs: map[string]string{"sportarr": "x"}}); err == nil {
		t.Fatal("metadata error")
	}
	if _, err := p.GetSeasons(context.Background(), metadata.SeasonsRequest{ProviderIDs: map[string]string{"sportarr": "x"}}); err == nil {
		t.Fatal("seasons error")
	}
	if _, err := p.GetEpisodes(context.Background(), metadata.EpisodesRequest{ProviderIDs: map[string]string{"sportarr": "x"}, SeasonNumber: 1}); err == nil {
		t.Fatal("episodes error")
	}
	if _, err := p.GetImages(context.Background(), metadata.ImageRequest{ProviderIDs: map[string]string{"sportarr": "x"}, ContentType: "series"}); err == nil {
		t.Fatal("images error")
	}
	imgs, err := p.GetImages(context.Background(), metadata.ImageRequest{ProviderIDs: map[string]string{"sportarr": "x"}, ContentType: "other"})
	if err != nil || imgs != nil {
		t.Fatalf("default images: %v %#v", err, imgs)
	}
}
