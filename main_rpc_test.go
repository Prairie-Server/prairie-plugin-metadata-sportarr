package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginv1 "github.com/prairie-server/prairie-plugin-sdk/pkg/pluginproto/prairie/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/prairie-server/prairie-plugin-metadata-sportarr/metadata"
	"github.com/prairie-server/prairie-plugin-metadata-sportarr/provider"
)

func testSportarrServers(t *testing.T) (*metadataServer, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/metadata/agents/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": "league-1", "title": "Premier League", "year": 1992, "overview": "Football"},
				},
			})
		case "/api/metadata/agents/series/league-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"title": "Premier League", "summary": "Football", "year": 1992, "genres": []string{"Sports"}, "studio": "FA",
			})
		case "/api/metadata/agents/series/league-1/seasons":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"seasons": []map[string]any{
					{"competition_season_id": "cs-2024", "season_number": 2024, "name": "2024", "overview": "Season", "air_date": "2024-01-01"},
				},
			})
		case "/api/metadata/agents/series/league-1/season/2024/episodes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"episodes": []map[string]any{
					{"id": "ev-1", "title": "Match 1", "season_number": 2024, "episode_number": 1, "overview": "Kickoff", "air_date": "2024-08-01", "duration_minutes": 90},
				},
			})
		case "/api/v1/images/entity/league/league-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"images": []map[string]any{
					{"id": "p1", "image_type": "poster", "url": "https://sportarr.net/api/v1/images/p1", "is_primary": true},
					{"id": "b1", "image_type": "backdrop", "url": "https://sportarr.net/api/v1/images/b1", "is_primary": true},
					{"id": "l1", "image_type": "logo", "url": "https://sportarr.net/api/v1/images/l1"},
					{"id": "bn1", "image_type": "banner", "url": "https://sportarr.net/api/v1/images/bn1"},
					{"id": "t1", "image_type": "thumbnail", "url": "https://sportarr.net/api/v1/images/t1"},
				},
			})
		case "/api/v1/images/entity/season/cs-2024":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"images": []map[string]any{
					{"id": "sp1", "image_type": "poster", "url": "https://sportarr.net/api/v1/images/sp1", "is_primary": true},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client := provider.NewClient(100)
	client.SetBaseURL(srv.URL)
	rt := &runtimeServer{
		provider: provider.NewProviderWithClient(client),
		baseURL:  srv.URL,
	}
	return &metadataServer{runtime: rt}, srv
}

func TestRuntimeGetManifestConfigureAndProvider(t *testing.T) {
	prev := version
	version = "3.2.1-test"
	t.Cleanup(func() { version = prev })

	m, err := loadManifest()
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if m.GetVersion() != "3.2.1-test" || m.GetChecksum() == "" {
		t.Fatalf("manifest = %#v", m)
	}

	rt := &runtimeServer{manifest: m}
	resp, err := rt.GetManifest(context.Background(), &pluginv1.GetManifestRequest{})
	if err != nil || resp.GetManifest() != m {
		t.Fatalf("GetManifest: %v", err)
	}
	if _, err := rt.providerForRequest(); err == nil {
		t.Fatal("expected not configured")
	}

	cfg, err := structpb.NewStruct(map[string]any{"base_url": "https://custom.example/"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Configure(context.Background(), &pluginv1.ConfigureRequest{
		Config: []*pluginv1.ConfigEntry{
			{Key: "other", Value: cfg},
			{Key: "sportarr", Value: cfg},
		},
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if rt.baseURL != "https://custom.example" || rt.provider == nil {
		t.Fatalf("baseURL=%q provider=%v", rt.baseURL, rt.provider)
	}
	if _, err := rt.Configure(context.Background(), &pluginv1.ConfigureRequest{}); err != nil {
		t.Fatalf("Configure default: %v", err)
	}
	if rt.baseURL != defaultBaseURL {
		t.Fatalf("default base = %q", rt.baseURL)
	}

	p := provider.NewProvider(rt.baseURL)
	if p.Name() != "Sportarr" || len(p.ForTypes()) != 1 {
		t.Fatalf("provider meta: %s %v", p.Name(), p.ForTypes())
	}
}

func TestMetadataServerRPCs(t *testing.T) {
	ms, srv := testSportarrServers(t)

	ids, err := structpb.NewStruct(map[string]any{"sportarr": "league-1"})
	if err != nil {
		t.Fatal(err)
	}

	search, err := ms.Search(context.Background(), &pluginv1.SearchMetadataRequest{
		Query: "Premier", ItemType: "series", Year: 1992, Language: "en", ProviderIds: ids,
	})
	if err != nil || len(search.GetResults()) != 1 {
		t.Fatalf("Search: %v %#v", err, search)
	}
	if search.GetResults()[0].GetTitle() != "Premier League" {
		t.Fatalf("%#v", search.GetResults()[0])
	}

	meta, err := ms.GetMetadata(context.Background(), &pluginv1.GetMetadataRequest{
		ProviderId: "league-1", ItemType: "series", ProviderIds: ids, Language: "en",
	})
	if err != nil || meta.GetItem() == nil || meta.GetItem().GetTitle() != "Premier League" {
		t.Fatalf("GetMetadata: %v %#v", err, meta)
	}

	if _, err := ms.GetPersonDetail(context.Background(), &pluginv1.GetPersonDetailRequest{}); err != nil {
		t.Fatalf("GetPersonDetail: %v", err)
	}

	seasons, err := ms.GetSeasons(context.Background(), &pluginv1.GetSeasonsRequest{
		SeriesProviderId: "league-1", ProviderIds: ids, Language: "en",
	})
	if err != nil || len(seasons.GetSeasons()) != 1 {
		t.Fatalf("GetSeasons: %v %#v", err, seasons)
	}

	episodes, err := ms.GetEpisodes(context.Background(), &pluginv1.GetEpisodesRequest{
		SeriesProviderId: "league-1", SeasonNumber: 2024, ProviderIds: ids, Language: "en",
	})
	if err != nil || len(episodes.GetEpisodes()) != 1 {
		t.Fatalf("GetEpisodes: %v %#v", err, episodes)
	}

	images, err := ms.GetImages(context.Background(), &pluginv1.GetImagesRequest{
		ProviderId: "league-1", ItemType: "series", ProviderIds: ids,
	})
	if err != nil || len(images.GetImages()) < 4 {
		t.Fatalf("GetImages: %v %#v", err, images)
	}

	path := "sportarr:///api/v1/images/p1"
	one, err := ms.ResolveImageURL(context.Background(), &pluginv1.ResolveImageURLRequest{Path: path, Variant: "card"})
	if err != nil || one.GetUrl() != srv.URL+"/api/v1/images/p1" {
		t.Fatalf("ResolveImageURL: %v %#v", err, one)
	}
	many, err := ms.ResolveImageURLs(context.Background(), &pluginv1.ResolveImageURLsRequest{
		Paths: []string{path, "https://example.com/x.jpg"}, Variant: "full",
	})
	if err != nil || many.GetUrls()[path] == "" {
		t.Fatalf("ResolveImageURLs: %v %#v", err, many)
	}

	bad := &metadataServer{runtime: &runtimeServer{}}
	if _, err := bad.Search(context.Background(), &pluginv1.SearchMetadataRequest{}); err == nil {
		t.Fatal("expected search configure error")
	}
	if _, err := bad.GetMetadata(context.Background(), &pluginv1.GetMetadataRequest{}); err == nil {
		t.Fatal("expected metadata configure error")
	}
	if _, err := bad.GetSeasons(context.Background(), &pluginv1.GetSeasonsRequest{}); err == nil {
		t.Fatal("expected seasons configure error")
	}
	if _, err := bad.GetEpisodes(context.Background(), &pluginv1.GetEpisodesRequest{}); err == nil {
		t.Fatal("expected episodes configure error")
	}
	if _, err := bad.GetImages(context.Background(), &pluginv1.GetImagesRequest{}); err == nil {
		t.Fatal("expected images configure error")
	}
}

func TestHelperProtoMappers(t *testing.T) {
	if len(stringMapFromStruct(nil)) != 0 {
		t.Fatal("nil")
	}
	st, err := structpb.NewStruct(map[string]any{"a": "1", "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := stringMapFromStruct(st); got["a"] != "1" || len(got) != 1 {
		t.Fatalf("%#v", got)
	}
	ids := providerIDsFromProto(nil, "sportarr", "x")
	if ids["sportarr"] != "x" {
		t.Fatalf("%#v", ids)
	}
	empty, err := stringStruct(nil)
	if err != nil || empty != nil {
		t.Fatal(empty, err)
	}
	blank, err := stringStruct(map[string]string{"a": ""})
	if err != nil || blank != nil {
		t.Fatal(blank, err)
	}
	ok, err := stringStruct(map[string]string{"a": "1"})
	if err != nil || ok.GetFields()["a"].GetStringValue() != "1" {
		t.Fatal(ok, err)
	}

	item, err := metadataItemFromResult(&metadata.MetadataResult{
		Title:         "T",
		OriginalTitle: "OT",
		SortTitle:     "ST",
		Year:          2000,
		Overview:      "O",
		ProviderIDs:   map[string]string{"sportarr": "id1"},
		PosterPath:    "https://sportarr.net/api/v1/images/p1",
	}, "series", "https://sportarr.net")
	if err != nil || item.GetTitle() != "T" || item.GetPosterPath() != "sportarr:///api/v1/images/p1" {
		t.Fatalf("%v %#v", err, item)
	}

	_ = metadataRequestFromProto(&pluginv1.GetMetadataRequest{ProviderId: "x", ItemType: "series"}, "sportarr")
	_ = seasonsRequestFromProto(&pluginv1.GetSeasonsRequest{SeriesProviderId: "x"}, "sportarr")
	_ = episodesRequestFromProto(&pluginv1.GetEpisodesRequest{SeriesProviderId: "x", SeasonNumber: 1}, "sportarr")
	_ = imageRequestFromProto(&pluginv1.GetImagesRequest{ProviderId: "x", ItemType: "series"}, "sportarr")
}

func TestMetadataServerPropagatesProviderErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fail", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	client := provider.NewClient(100)
	client.SetBaseURL(srv.URL)
	ms := &metadataServer{runtime: &runtimeServer{provider: provider.NewProviderWithClient(client), baseURL: srv.URL}}
	ids, _ := structpb.NewStruct(map[string]any{"sportarr": "league-1"})

	if _, err := ms.Search(context.Background(), &pluginv1.SearchMetadataRequest{Query: "x"}); err == nil {
		t.Fatal("search")
	}
	if _, err := ms.GetMetadata(context.Background(), &pluginv1.GetMetadataRequest{ProviderId: "league-1", ProviderIds: ids}); err == nil {
		t.Fatal("metadata")
	}
	if _, err := ms.GetSeasons(context.Background(), &pluginv1.GetSeasonsRequest{SeriesProviderId: "league-1", ProviderIds: ids}); err == nil {
		t.Fatal("seasons")
	}
	if _, err := ms.GetEpisodes(context.Background(), &pluginv1.GetEpisodesRequest{SeriesProviderId: "league-1", SeasonNumber: 1, ProviderIds: ids}); err == nil {
		t.Fatal("episodes")
	}
	if _, err := ms.GetImages(context.Background(), &pluginv1.GetImagesRequest{ProviderId: "league-1", ItemType: "series", ProviderIds: ids}); err == nil {
		t.Fatal("images")
	}
}
